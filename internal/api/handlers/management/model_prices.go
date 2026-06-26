package management

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fxzer/cpa-core/v7/internal/requestevents"
	"github.com/gin-gonic/gin"
)

const modelPriceSyncSource = "litellm"

var modelPriceSyncURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

func (h *Handler) GetModelPrices(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	prices, err := store.LoadModelPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

func (h *Handler) PutModelPrices(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	var req struct {
		Prices map[string]requestevents.ModelPrice `json:"prices"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Prices == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prices are required"})
		return
	}
	if err := store.SaveModelPrices(c.Request.Context(), req.Prices); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prices, err := store.LoadModelPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

func (h *Handler) SyncModelPrices(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	var req struct {
		Models []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	remotePrices, skipped, err := fetchLiteLLMModelPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	selectedPrices := selectModelPrices(remotePrices, req.Models)
	result, err := store.UpsertSyncedModelPrices(c.Request.Context(), selectedPrices)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	prices, err := store.LoadModelPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"source":   modelPriceSyncSource,
		"imported": result.Imported,
		"skipped":  result.Skipped + skipped,
		"prices":   prices,
	})
}

func fetchLiteLLMModelPrices(ctx context.Context) (map[string]requestevents.ModelPrice, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelPriceSyncURL, nil)
	if err != nil {
		return nil, 0, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, 0, errors.New("model price sync failed: " + res.Status)
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, 0, err
	}

	prices := map[string]requestevents.ModelPrice{}
	skipped := 0
	for model, raw := range payload {
		if model == "" || model == "sample_spec" {
			skipped++
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(raw, &entry); err != nil {
			skipped++
			continue
		}

		prompt, hasPrompt := readFloat(entry, "input_cost_per_token")
		completion, hasCompletion := readFloat(entry, "output_cost_per_token")
		cache, hasCache := readFloat(entry, "cache_read_input_token_cost")
		if !hasCache {
			cache, hasCache = readFloat(entry, "cache_read_cost_per_token")
		}
		if !hasPrompt && !hasCompletion {
			skipped++
			continue
		}
		if !hasPrompt {
			prompt = 0
		}
		if !hasCompletion {
			completion = 0
		}
		if !hasCache {
			cache = prompt
		}

		prices[model] = requestevents.ModelPrice{
			Prompt:        prompt * 1_000_000,
			Completion:    completion * 1_000_000,
			Cache:         cache * 1_000_000,
			Source:        modelPriceSyncSource,
			SourceModelID: model,
			RawJSON:       string(raw),
		}
	}
	return prices, skipped, nil
}

func selectModelPrices(prices map[string]requestevents.ModelPrice, models []string) map[string]requestevents.ModelPrice {
	wanted := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		wanted = append(wanted, model)
	}
	if len(wanted) == 0 {
		return prices
	}

	selected := map[string]requestevents.ModelPrice{}
	for _, model := range wanted {
		if price, ok := prices[model]; ok {
			selected[model] = price
			continue
		}
		if price, ok := findSuffixModelPrice(prices, model); ok {
			selected[model] = price
		}
	}
	return selected
}

func findSuffixModelPrice(prices map[string]requestevents.ModelPrice, model string) (requestevents.ModelPrice, bool) {
	var bestKey string
	for key := range prices {
		if strings.HasSuffix(model, key) && len(key) > len(bestKey) {
			bestKey = key
		}
	}
	if bestKey == "" {
		return requestevents.ModelPrice{}, false
	}
	return prices[bestKey], true
}

func readFloat(record map[string]any, key string) (float64, bool) {
	raw, ok := record[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
