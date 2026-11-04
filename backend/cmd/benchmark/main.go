// Command benchmark compara modelos de Gemini en reconocimiento de comida
// usando fotos reales de Nutrition5k con gramos pesados en balanza.
//
//	go run ./cmd/benchmark -n 100
//
// Lee GEMINI_API_KEY del entorno o de backend/.env.
package main

import (
	"github.com/whiteowltech/owli/backend/internal/envfile"

	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/whiteowltech/owli/backend/internal/adapters/anthropic"
	"github.com/whiteowltech/owli/backend/internal/adapters/gemini"
	"github.com/whiteowltech/owli/backend/internal/ports"
)

type result struct {
	Model     string
	DishID    string
	TruthMass float64
	PredMass  float64
	MassErr   float64
	Recall    float64
	Items     int
	Review    bool
	Latency   time.Duration // del intento exitoso
	Retries   int           // reintentos por 429/503 antes de responder
	Usage     ports.TokenUsage
	Cost      float64
	Err       error
	Predicted string
	Truth     string
}

func main() {
	var (
		n           = flag.Int("n", 100, "cantidad de platos")
		models      = flag.String("models", "gemini-3.1-flash-lite,gemini-3.5-flash-lite,gemini-3.8-flash", "modelos separados por coma")
		seed        = flag.Int64("seed", 42, "semilla del muestreo (reproducible)")
		concurrency = flag.Int("concurrency", 4, "peticiones simultáneas")
		cacheDir    = flag.String("cache", ".bench-cache", "directorio de caché del dataset y resultados")
		thinking    = flag.String("thinking", "", "thinkingLevel opcional (minimal, low, medium, high)")
	)
	flag.Parse()

	if err := envfile.Load(".env"); err != nil {
		log.Fatalf("leer .env: %v", err)
	}
	modelList := strings.Split(*models, ",")
	for i := range modelList {
		modelList[i] = strings.TrimSpace(modelList[i])
	}
	var clients []ports.FoodRecognizer
	for _, m := range modelList {
		c, err := newRecognizer(m, *thinking)
		if err != nil {
			log.Fatal(err)
		}
		clients = append(clients, c)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Printf("cargando metadatos de Nutrition5k…")
	all, err := loadDishes(ctx, *cacheDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d platos en el dataset; descargando %d fotos…", len(all), *n)
	dishes, err := sampleDishes(ctx, all, *n, *seed, *cacheDir)
	if err != nil {
		log.Fatal(err)
	}

	results := run(ctx, clients, modelList, dishes, *concurrency)
	printSummary(results, modelList)

	out := filepath.Join(*cacheDir, fmt.Sprintf("results-%s.csv", time.Now().Format("20060102-150405")))
	if err := writeCSV(out, results); err != nil {
		log.Fatal(err)
	}
	log.Printf("detalle por plato: %s", out)
}

// newRecognizer elige el proveedor por el prefijo del modelo.
func newRecognizer(model, thinking string) (ports.FoodRecognizer, error) {
	switch {
	case strings.HasPrefix(model, "gemini-"):
		key := os.Getenv("GEMINI_API_KEY")
		if key == "" {
			return nil, errors.New("falta GEMINI_API_KEY (en el entorno o en .env)")
		}
		return gemini.New(gemini.Config{APIKey: key, Model: model, ThinkingLevel: thinking})
	case strings.HasPrefix(model, "claude-"):
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, errors.New("falta ANTHROPIC_API_KEY (en el entorno o en .env)")
		}
		return anthropic.New(anthropic.Config{APIKey: key, Model: model})
	}
	return nil, fmt.Errorf("proveedor desconocido para el modelo %q", model)
}

func run(ctx context.Context, clients []ports.FoodRecognizer, models []string, dishes []dish, concurrency int) []result {
	var (
		mu      sync.Mutex
		results []result
		wg      sync.WaitGroup
		sem     = make(chan struct{}, concurrency)
		done    int
		total   = len(clients) * len(dishes)
	)
	for i, c := range clients {
		model := strings.TrimSpace(models[i])
		for _, d := range dishes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				r := evaluate(ctx, c, model, d)
				mu.Lock()
				results = append(results, r)
				done++
				if done%10 == 0 || done == total {
					log.Printf("%d/%d", done, total)
				}
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return results
}

func evaluate(ctx context.Context, c ports.FoodRecognizer, model string, d dish) result {
	r := result{Model: model, DishID: d.ID, TruthMass: d.TotalMass}
	var truth []string
	for _, ing := range d.Ingredients {
		truth = append(truth, fmt.Sprintf("%s:%.0f", ing.Name, ing.Grams))
	}
	r.Truth = strings.Join(truth, "; ")

	var res ports.RecognitionResult
	var err error
	backoff := 2 * time.Second
	for attempt := 0; attempt < 6; attempt++ {
		start := time.Now()
		res, err = c.RecognizeImage(ctx, ports.Image{Data: d.JPEG, MIMEType: "image/jpeg"})
		r.Latency = time.Since(start)
		if !ports.IsRetryable(err) {
			break
		}
		r.Retries++
		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			r.Err = ctx.Err()
			return r
		}
	}
	if err != nil {
		r.Err = err
		return r
	}

	rec := res.Recognition
	r.PredMass = rec.TotalGrams()
	r.MassErr = massErrorPct(rec, d.TotalMass)
	r.Recall = weightedRecall(rec, d.Ingredients)
	r.Items = len(rec.Items)
	r.Review = rec.NeedsReview()
	r.Usage = res.Usage
	r.Cost = costUSD(model, res.Usage)
	var pred []string
	for _, it := range rec.Items {
		pred = append(pred, fmt.Sprintf("%s:%.0f(%.2f)", it.Name, it.Grams, it.Confidence))
	}
	r.Predicted = strings.Join(pred, "; ")
	return r
}

func printSummary(results []result, models []string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "modelo\tok\terrores\treintentos\terror masa mediana %\terror masa medio %\trecall ponderado %\trevisión %\tlatencia p50\tlatencia p95\ttokens in/out\tUSD/foto\tUSD/1000 fotos\t")
	for _, m := range models {
		m = strings.TrimSpace(m)
		var massErr, recall, lat, cost, in, out []float64
		var errs, review, retries int
		for _, r := range results {
			if r.Model != m {
				continue
			}
			retries += r.Retries
			if r.Err != nil {
				errs++
				continue
			}
			massErr = append(massErr, r.MassErr)
			recall = append(recall, r.Recall)
			lat = append(lat, float64(r.Latency.Milliseconds()))
			cost = append(cost, r.Cost)
			in = append(in, float64(r.Usage.InputTokens))
			out = append(out, float64(r.Usage.OutputTokens))
			if r.Review {
				review++
			}
		}
		ok := len(massErr)
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%.1f\t%.1f\t%.1f\t%.0f\t%.0f ms\t%.0f ms\t%.0f/%.0f\t$%.5f\t$%.2f\t\n",
			m, ok, errs, retries,
			percentile(massErr, 50), mean(massErr), mean(recall),
			float64(review)/float64(max(ok, 1))*100,
			percentile(lat, 50), percentile(lat, 95),
			mean(in), mean(out), mean(cost), mean(cost)*1000)
	}
	w.Flush()

	for _, r := range results {
		if r.Err != nil {
			fmt.Printf("ERROR %s %s: %v\n", r.Model, r.DishID, r.Err)
		}
	}
}

func writeCSV(path string, results []result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"model", "dish_id", "truth_mass_g", "pred_mass_g", "mass_err_pct", "weighted_recall_pct", "items", "needs_review", "latency_ms", "retries", "input_tokens", "output_tokens", "thinking_tokens", "cost_usd", "error", "predicted", "truth"})
	for _, r := range results {
		errStr := ""
		if r.Err != nil {
			errStr = r.Err.Error()
		}
		_ = w.Write([]string{
			r.Model, r.DishID,
			ff(r.TruthMass), ff(r.PredMass), ff(r.MassErr), ff(r.Recall),
			strconv.Itoa(r.Items), strconv.FormatBool(r.Review),
			strconv.FormatInt(r.Latency.Milliseconds(), 10), strconv.Itoa(r.Retries),
			strconv.Itoa(r.Usage.InputTokens), strconv.Itoa(r.Usage.OutputTokens), strconv.Itoa(r.Usage.ThinkingTokens),
			strconv.FormatFloat(r.Cost, 'f', 6, 64),
			errStr, r.Predicted, r.Truth,
		})
	}
	w.Flush()
	return w.Error()
}

func ff(x float64) string { return strconv.FormatFloat(x, 'f', 1, 64) }
