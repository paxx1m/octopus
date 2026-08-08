package helper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/dlclark/regexp2"
	"github.com/looplj/axonhub/llm"
)

func FetchModels(ctx context.Context, request model.Channel) ([]string, error) {
	client, err := ChannelHttpClient(&request)
	if err != nil {
		return nil, err
	}
	fetchModel := make([]string, 0)
	switch request.Type {
	case llm.APIFormatAnthropicMessage:
		fetchModel, err = fetchAnthropicModels(client, ctx, request)
	case llm.APIFormatGeminiContents:
		fetchModel, err = fetchGeminiModels(client, ctx, request)
	default:
		fetchModel, err = fetchOpenAIModels(client, ctx, request)
	}
	if err != nil {
		return nil, err
	}
	if request.MatchRegex != nil && *request.MatchRegex != "" {
		matchModel := make([]string, 0)
		re, err := regexp2.Compile(*request.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			return nil, err
		}
		for _, model := range fetchModel {
			matched, err := re.MatchString(model)
			if err != nil {
				return nil, err
			}
			if matched {
				matchModel = append(matchModel, model)
			}
		}
		return matchModel, nil
	}
	return fetchModel, nil
}

func doJSON(client *http.Client, req *http.Request, dest any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// modelsBaseURL builds the base URL used for model-list requests.
// Unlike transformer.NormalizeBaseURL (always appends version for bare paths),
// this only appends version when the URL has no path (host-only).
// URLs that already include a path (e.g. /api/v1, /api/ollama, /api/nvidia)
// are left as-is and only have trailing slashes trimmed.
// A trailing "#" still forces raw mode (no version append), matching transformer convention.
func modelsBaseURL(raw, version string) string {
	if raw == "" {
		return ""
	}
	if before, ok := strings.CutSuffix(raw, "#"); ok {
		return strings.TrimRight(before, "/")
	}
	trimmed := strings.TrimRight(raw, "/")
	if version == "" {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/"+version) {
		return trimmed
	}
	if strings.Contains(trimmed, "/"+version+"/") {
		return trimmed
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Path == "" || u.Path == "/" {
		return trimmed + "/" + version
	}
	return trimmed
}

// refer: https://platform.openai.com/docs/api-reference/models/list
func fetchOpenAIModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	version := "v1"
	if request.Type == model.ChannelTypeDoubao {
		version = "v3"
	}
	baseURL := modelsBaseURL(request.GetBaseUrl(), version)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/models",
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+request.GetChannelKey().ChannelKey)
	applyCustomHeaders(req, request)

	var result model.OpenAIModelList
	if err := doJSON(client, req, &result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// refer: https://ai.google.dev/api/models
func fetchGeminiModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	var allModels []string
	pageToken := ""
	// Bare host → /v1beta; path already present (incl. explicit /v1) → keep as-is.
	baseURL := modelsBaseURL(request.GetBaseUrl(), "v1beta")

	for {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			baseURL+"/models",
			nil,
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Goog-Api-Key", request.GetChannelKey().ChannelKey)
		applyCustomHeaders(req, request)
		if pageToken != "" {
			q := req.URL.Query()
			q.Add("pageToken", pageToken)
			req.URL.RawQuery = q.Encode()
		}

		var result model.GeminiModelList
		if err := doJSON(client, req, &result); err != nil {
			return nil, err
		}

		for _, m := range result.Models {
			name := strings.TrimPrefix(m.Name, "models/")
			allModels = append(allModels, name)
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, request)
	}
	return allModels, nil
}

// refer: https://platform.claude.com/docs
func fetchAnthropicModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	var allModels []string
	var afterID string
	baseURL := modelsBaseURL(request.GetBaseUrl(), "v1")
	for {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			baseURL+"/models",
			nil,
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Api-Key", request.GetChannelKey().ChannelKey)
		req.Header.Set("Anthropic-Version", "2023-06-01")
		applyCustomHeaders(req, request)
		q := req.URL.Query()
		if afterID != "" {
			q.Set("after_id", afterID)
		}
		req.URL.RawQuery = q.Encode()

		var result model.AnthropicModelList
		if err := doJSON(client, req, &result); err != nil {
			return nil, err
		}

		for _, m := range result.Data {
			allModels = append(allModels, m.ID)
		}

		if !result.HasMore {
			break
		}

		afterID = result.LastID
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, request)
	}
	return allModels, nil
}

func applyCustomHeaders(req *http.Request, channel model.Channel) {
	for _, header := range channel.CustomHeader {
		if header.HeaderKey != "" {
			req.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}
}
