package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// Nutrition5k (Google Research, CC BY 4.0): platos reales de cafetería con
// cada ingrediente pesado en balanza. Thames et al., CVPR 2021.
const n5kBaseURL = "https://storage.googleapis.com/nutrition5k_dataset/nutrition5k_dataset"

var errNoImage = errors.New("el plato no tiene foto cenital")

type ingredient struct {
	Name  string
	Grams float64
}

type dish struct {
	ID          string
	TotalMass   float64
	Ingredients []ingredient
	JPEG        []byte
}

// loadDishes lee los metadatos de ambas cafeterías (con caché local).
func loadDishes(ctx context.Context, cacheDir string) ([]dish, error) {
	var all []dish
	for _, name := range []string{"dish_metadata_cafe1.csv", "dish_metadata_cafe2.csv"} {
		raw, err := cachedGet(ctx, n5kBaseURL+"/metadata/"+name, filepath.Join(cacheDir, name))
		if err != nil {
			return nil, err
		}
		r := csv.NewReader(bytes.NewReader(raw))
		r.FieldsPerRecord = -1 // filas de largo variable: 6 columnas + 7 por ingrediente
		rows, err := r.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("parsear %s: %w", name, err)
		}
		for _, row := range rows {
			if len(row) < 13 {
				continue
			}
			mass, err := strconv.ParseFloat(row[2], 64)
			if err != nil {
				continue
			}
			d := dish{ID: row[0], TotalMass: mass}
			for i := 6; i+6 < len(row); i += 7 {
				g, err := strconv.ParseFloat(row[i+2], 64)
				if err != nil {
					continue
				}
				d.Ingredients = append(d.Ingredients, ingredient{Name: row[i+1], Grams: g})
			}
			all = append(all, d)
		}
	}
	return all, nil
}

// sampleDishes elige n platos al azar (reproducible por semilla) que tengan
// foto y una masa razonable, y descarga sus fotos.
func sampleDishes(ctx context.Context, all []dish, n int, seed int64, cacheDir string) ([]dish, error) {
	rnd := rand.New(rand.NewSource(seed))
	rnd.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })

	var out []dish
	for _, d := range all {
		if len(out) == n {
			break
		}
		if d.TotalMass < 50 || d.TotalMass > 1500 {
			continue
		}
		img, err := loadImage(ctx, d.ID, cacheDir)
		if errors.Is(err, errNoImage) {
			continue
		}
		if err != nil {
			return nil, err
		}
		d.JPEG = img
		out = append(out, d)
	}
	if len(out) < n {
		return nil, fmt.Errorf("solo se encontraron %d platos con foto", len(out))
	}
	return out, nil
}

// loadImage descarga rgb.png y lo recomprime a JPEG calidad 80, igual que la app.
func loadImage(ctx context.Context, dishID, cacheDir string) ([]byte, error) {
	path := filepath.Join(cacheDir, "img", dishID+".jpg")
	if b, err := os.ReadFile(path); err == nil {
		return b, nil
	}
	raw, err := httpGet(ctx, n5kBaseURL+"/imagery/realsense_overhead/"+dishID+"/rgb.png")
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decodificar %s: %w", dishID, err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, fmt.Errorf("codificar %s: %w", dishID, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return buf.Bytes(), os.WriteFile(path, buf.Bytes(), 0o644)
}

func cachedGet(ctx context.Context, url, path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		return b, nil
	}
	b, err := httpGet(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return b, os.WriteFile(path, b, 0o644)
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, errNoImage
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, res.StatusCode)
	}
	return io.ReadAll(res.Body)
}
