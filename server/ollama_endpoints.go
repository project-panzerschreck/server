package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/ssestream"
	ollamatypes "github.com/wk-y/rama-swap/server/ollama-types"
	"github.com/wk-y/rama-swap/server/scheduler"
)

func (s *Server) ollamaTags(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)

	ramaModels, err := s.ramalama.GetModels()
	if err != nil {
		log.Printf("Failed to get models: %v\n", ramaModels)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("E_MODEL_GET\n"))
	}

	var models struct {
		Models []ollamatypes.Model `json:"models"`
	}
	for _, ramaModel := range ramaModels {
		model := ollamatypes.Model{
			Name:       ramaModel.Name,
			Model:      ramaModel.Name,
			ModifiedAt: ramaModel.Modified,
			Size:       ramaModel.Size,
		}

		info, err := s.ramalama.Inspect(ramaModel.Name)
		if err != nil {
			log.Printf("Failed to inspect full details of model %s: %v\n", ramaModel.Name, err)
		} else {
			model.Name = info.Name
			model.Details.Format = strings.ToLower(info.Format)
			if info.Metadata.GeneralArchitecture != nil {
				model.Details.Family = *info.Metadata.GeneralArchitecture
				model.Details.Families = []string{*info.Metadata.GeneralArchitecture}
			}

			if info.Metadata.GeneralSizeLabel != nil {
				model.Details.ParameterSize = *info.Metadata.GeneralSizeLabel
			}
		}

		models.Models = append(models.Models, model)
	}

	w.Header().Add("Content-Type", "application/json; charset=utf-8")

	err = json.NewEncoder(w).Encode(models)
	if err != nil {
		log.Printf("Failed to reply: %v\n", err)
	}
}

func (s *Server) ollamaVersion(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)

	var version struct {
		Version string `json:"version"`
	}

	version.Version = "0.11.10" // arbitrary

	w.Header().Add("Content-Type", "application/json; charset=utf-8")

	err := json.NewEncoder(w).Encode(version)
	if err != nil {
		log.Printf("Failed to reply: %v\n", err)
	}
}

func (s *Server) ollamaChat(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)

	var requestJson ollamatypes.ChatRequest
	requestJson.Stream = true // default value

	rDecoder := json.NewDecoder(r.Body)
	err := rDecoder.Decode(&requestJson)
	if err != nil || requestJson.Model == nil {
		log.Println("Bad chat request:", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request JSON\n"))
		return
	}

	model := *requestJson.Model

	backendModel, err := s.scheduler.Lock(r.Context(), model)
	if err != nil {
		if errors.Is(err, scheduler.ErrModelLoading) {
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("model is loading, please retry\n"))
			return
		}
		log.Printf("Failed to start model %s: %v\n", model, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("E_MODEL_START\n"))
		return
	}
	defer s.scheduler.Unlock(backendModel)

	params, err := ollamaTranslateParams(requestJson)
	if err != nil {
		log.Printf("Failed to translate request: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("E_COMPLETION_TRANSLATE"))
	}

	var stream *ssestream.Stream[openai.ChatCompletionChunk]
	err = backendModel.WithClient(func(client openai.Client) error {
		stream = client.Chat.Completions.NewStreaming(r.Context(), params)
		return nil
	})
	if err != nil {
		log.Println("Error connecting to backend:", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("E_BACKEND_CONNECT\n"))
		return
	}

	completionStartTime := time.Now().UTC()

	defer stream.Close()

	responseEncoder := json.NewEncoder(w)
	responseController := http.NewResponseController(w)

	// only written to in non-streaming mode to send all tokens in the final response
	var accumulator strings.Builder

	// time the first token was generated
	var firstCreatedAt *time.Time

	// number of tokens generated
	var evalCount int64

	for stream.Next() {
		event := stream.Current()
		if len(event.Choices) > 0 {
			evalCount++

			if firstCreatedAt == nil {
				time := time.Unix(event.Created, 0)
				firstCreatedAt = &time
			}

			if requestJson.Stream == false {
				accumulator.WriteString(event.Choices[0].Delta.Content)
				continue
			}

			ollamaEvent := ollamatypes.ChatResponse{
				Model:     model,
				CreatedAt: time.Unix(event.Created, 0).Format(time.RFC3339),
				Message: ollamatypes.Message{
					Role:    "assistant",
					Content: event.Choices[0].Delta.Content,
				},
				Done: false,
			}

			err = responseEncoder.Encode(ollamaEvent)
			if err != nil {
				log.Printf("Failed to send delta: %v\n", err)
				return
			}

			// Flush to reduce stream latency. Whether it succeeds isn't important.
			_ = responseController.Flush()
		}
	}

	if err := stream.Err(); err != nil {
		log.Println("Error during response stream:", err)
		// keep going to send final response
	}

	completionFinishTime := time.Now().UTC()

	if firstCreatedAt == nil {
		firstCreatedAt = &completionFinishTime
	}

	responseEncoder.Encode(ollamatypes.ChatFinalResponse{
		ChatResponse: ollamatypes.ChatResponse{
			Model:     model,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Message: ollamatypes.Message{
				Role:    "assistant",
				Content: accumulator.String(),
			},
			Done: true,
		},
		TotalDuration: completionFinishTime.Sub(completionStartTime).Nanoseconds(),
		EvalDuration:  completionFinishTime.Sub(*firstCreatedAt).Nanoseconds(),
		EvalCount:     evalCount,
	})
}

// ollamaTranslateParams translates an ollama request into an openai chat completion request.
// Images attached to non-user messages are not supported and will be silently ignored.
func ollamaTranslateParams(request ollamatypes.ChatRequest) (completion openai.ChatCompletionNewParams, err error) {
	completion.Model = *request.Model

	completion.Messages = make([]openai.ChatCompletionMessageParamUnion, len(request.Messages))
	for i, message := range request.Messages {
		switch message.Role {
		case "user":
			if len(message.Images) == 0 {
				completion.Messages[i] = openai.UserMessage(message.Content)
				break
			}

			var content []openai.ChatCompletionContentPartUnionParam

			for _, img := range message.Images {
				uri, err := base64ToImageUri(img)
				if err != nil {
					return completion, err
				}

				content = append(content,
					openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
						URL: uri,
					}),
				)
			}

			content = append(content, openai.TextContentPart(message.Content))
			completion.Messages[i] = openai.UserMessage(content)

		case "system":
			completion.Messages[i] = openai.SystemMessage(message.Content)
		default: // fallback to assistant message type
			completion.Messages[i] = openai.AssistantMessage(message.Content)
		}
	}

	if request.Options != nil {
		ollamaAddOptions(&completion, *request.Options)
	}

	return
}

func base64ToImageUri(img string) (string, error) {
	_, format, err := image.DecodeConfig(base64.NewDecoder(base64.StdEncoding, strings.NewReader(img)))
	if err != nil {
		return "", fmt.Errorf("failed to determine image type: %v", err)
	}

	switch format {
	case "png":
		return "data:image/png;base64," + img, nil
	case "jpeg":
		return "data:image/jpeg;base64," + img, nil
	case "gif":
		return "data:image/gif;base64," + img, nil
	default:
		return "", fmt.Errorf("decodable but unsupported image format %s", format)
	}
}

// ollamaAddOptions sets completion's options to ollama options, where possible.
// Options that are not set will not be changed.
func ollamaAddOptions(completion *openai.ChatCompletionNewParams, options ollamatypes.Options) {
	if options.Temperature != nil {
		completion.Temperature = openai.Opt(*options.Temperature)
	}

	if options.Seed != nil {
		completion.Seed = openai.Opt(*options.Seed)
	}

	if options.NumPredict != nil {
		completion.MaxCompletionTokens = openai.Opt(*options.NumPredict)
	}

	if options.TopP != nil {
		completion.TopP = openai.Opt(*options.TopP)
	}
}
