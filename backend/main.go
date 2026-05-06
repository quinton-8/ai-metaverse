package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

// Message matches the format sent by the frontend
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Messages []Message `json:"messages"`
	ModelID  string    `json:"modelId"`
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", handleChat)

	// Apply CORS middleware
	handler := corsMiddleware(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Go Backend server listening on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

// corsMiddleware allows the Next.js app (usually on port 3000) to communicate with Go
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // Change "*" to "http://localhost:3000" in production
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set headers for Server-Sent Events (SSE) which useChat expects
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()

	// Switch the AI model based on the payload
	switch req.ModelID {
	case "gpt-4o", "gpt-3.5-turbo":
		streamOpenAI(ctx, req, w, flusher)
	case "gemini-1.5-pro":
		streamGemini(ctx, req, w, flusher)
	default:
		// Fallback to OpenAI
		streamOpenAI(ctx, req, w, flusher)
	}
}

// --- OPENAI IMPLEMENTATION ---
func streamOpenAI(ctx context.Context, req ChatRequest, w http.ResponseWriter, flusher http.Flusher) {
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Convert frontend messages to OpenAI format
	var oaiMessages []openai.ChatCompletionMessage
	for _, m := range req.Messages {
		oaiMessages = append(oaiMessages, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	stream, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    req.ModelID,
		Messages: oaiMessages,
		Stream:   true,
	})
	if err != nil {
		fmt.Fprintf(w, "e:%q\n", err.Error())
		flusher.Flush()
		return
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			fmt.Fprintf(w, "e:%q\n", err.Error())
			flusher.Flush()
			return
		}

		// Format chunk exactly how the Vercel AI SDK expects it: 0:"text chunk"\n
		text := response.Choices[0].Delta.Content
		if text != "" {
			chunkData, _ := json.Marshal(text)
			fmt.Fprintf(w, "0:%s\n", chunkData)
			flusher.Flush()
		}
	}
}

// --- GEMINI IMPLEMENTATION ---
func streamGemini(ctx context.Context, req ChatRequest, w http.ResponseWriter, flusher http.Flusher) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		fmt.Fprintf(w, "e:%q\n", err.Error())
		flusher.Flush()
		return
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash")

	model.SystemInstruction = &genai.Content {
		Parts: []genai.Part {
			genai.Text("You are an expert Micro-Bussiness Marketing Consultant. Provide actionable, low-cost marketing strategies for small businesses."),
		},
	}

	cs := model.StartChat()

	// Populate Gemini chat history (skipping the last message which is the current prompt)
	for i := 0; i < len(req.Messages)-1; i++ {
		m := req.Messages[i]
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		cs.History = append(cs.History, &genai.Content{
			Parts: []genai.Part{genai.Text(m.Content)},
			Role:  role,
		})
	}

	// Send the final message to trigger the stream
	lastMessage := req.Messages[len(req.Messages)-1].Content
	iter := cs.SendMessageStream(ctx, genai.Text(lastMessage))

	for {
		resp, err := iter.Next()
		if err == genai.Done {
			break
		}
		if err != nil {
			fmt.Fprintf(w, "e:%q\n", err.Error())
			flusher.Flush()
			return
		}

		// Extract text from Gemini parts and format for Vercel AI SDK
		for _, part := range resp.Candidates[0].Content.Parts {
			if text, ok := part.(genai.Text); ok {
				chunkData, _ := json.Marshal(string(text))
				fmt.Fprintf(w, "0:%s\n", chunkData)
				flusher.Flush()
			}
		}
	}
}
