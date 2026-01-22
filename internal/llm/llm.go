/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package llm

/*
#cgo LDFLAGS: -lzigllm
#include <llm.h>
#include <stdlib.h>
#include <string.h>

void GoHttpClientCallbackPdf(
	void* allocator_handle,
	char* method,
	char* url,
	char* headers,
	void* body,
	uint32_t body_length,
	char** response_body_output,
	uint32_t* response_body_length_output,
	uint16_t* status_output
);
*/
import "C"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unsafe"

	"github.com/book-expert/logger"
)

const (
	RetryDelay      = 2 * time.Second
	UploadURLHeader = "X-Goog-Upload-URL"
	DefaultTimeout  = 90 * time.Second
)

type Config struct {
	APIKey          string
	BaseAddress     string
	Model           string
	Temperature     float64
	MaxOutputTokens int
	TimeoutSeconds  int
	MaxRetries      int
}

type Client struct {
	handle C.zig_llm_handle_t
	logger *logger.Logger
	config Config
}

var httpClient = &http.Client{
	Timeout: DefaultTimeout,
}

func NewClient(_ context.Context, configuration *Config, serviceLogger *logger.Logger) (*Client, error) {
	apiKey := unsafe.Pointer(unsafe.StringData(configuration.APIKey))
	model := unsafe.Pointer(unsafe.StringData(configuration.Model))
	baseAddress := unsafe.Pointer(unsafe.StringData(configuration.BaseAddress))

	zigConfig := C.zig_llm_config_t{
		api_key:           apiKey,
		api_key_length:    C.uint32_t(len(configuration.APIKey)),
		model:             model,
		model_length:      C.uint32_t(len(configuration.Model)),
		base_url:          baseAddress,
		base_url_length:   C.uint32_t(len(configuration.BaseAddress)),
		max_output_tokens: C.int32_t(configuration.MaxOutputTokens),
		temperature:       C.float(configuration.Temperature),
		http_callback:     (C.zig_llm_http_callback)(unsafe.Pointer(C.GoHttpClientCallbackPdf)),
	}

	handle := C.zig_llm_init(zigConfig)
	if handle == nil {
		return nil, errors.New("failed to initialize Zig LLM client")
	}

	return &Client{
		handle: handle,
		config: *configuration,
		logger: serviceLogger,
	}, nil
}

//export GoHttpClientCallbackPdf
func GoHttpClientCallbackPdf(
	allocatorHandle unsafe.Pointer,
	method *C.char,
	url *C.char,
	headers *C.char,
	body unsafe.Pointer,
	bodyLength C.uint32_t,
	responseBodyOutput **C.char,
	responseBodyLengthOutput *C.uint32_t,
	statusOutput *C.uint16_t,
) {
	goMethod := C.GoString(method)
	goUrl := C.GoString(url)
	goHeaders := C.GoString(headers)
	goBody := C.GoBytes(body, C.int(bodyLength))

	request, error := http.NewRequest(goMethod, goUrl, bytes.NewReader(goBody))
	if error != nil {
		*statusOutput = 500
		return
	}

	lines := strings.Split(goHeaders, "\r\n")
	for _, line := range lines {
		if parts := strings.SplitN(line, ": ", 2); len(parts) == 2 {
			request.Header.Set(parts[0], parts[1])
		}
	}

	response, error := httpClient.Do(request)
	if error != nil {
		*statusOutput = 500
		return
	}
	defer func() { _ = response.Body.Close() }()

	*statusOutput = C.uint16_t(response.StatusCode)

	var resultBody []byte
	if response.Header.Get(UploadURLHeader) != "" {
		resultBody = []byte(response.Header.Get(UploadURLHeader))
	} else {
		resultBody, _ = io.ReadAll(response.Body)
	}

	*responseBodyLengthOutput = C.uint32_t(len(resultBody))
	if len(resultBody) > 0 {
		cBody := C.zig_llm_alloc(allocatorHandle, C.uint32_t(len(resultBody)))
		if cBody != nil {
			C.memcpy(cBody, unsafe.Pointer(&resultBody[0]), C.size_t(len(resultBody)))
			*responseBodyOutput = (*C.char)(cBody)
		}
	}
}

func (client *Client) Close() {
	if client.handle != nil {
		C.zig_llm_deinit(client.handle)
		client.handle = nil
	}
}

func (client *Client) GenerateContent(parentContext context.Context, systemInstruction, userPrompt string, data []byte, mimeType string) (string, error) {
	var lastError error

	for attempt := 1; attempt <= client.config.MaxRetries; attempt++ {
		result, callError := client.callZigLLM(parentContext, systemInstruction, userPrompt, data, mimeType)
		if callError == nil {
			return result, nil
		}

		lastError = callError
		client.logger.Warnf("LLM attempt %d/%d failed: %v", attempt, client.config.MaxRetries, callError)

		if attempt < client.config.MaxRetries {
			select {
			case <-parentContext.Done():
				return "", parentContext.Err()
			case <-time.After(RetryDelay):
				continue
			}
		}
	}

	return "", fmt.Errorf("all %d attempts failed: %w", client.config.MaxRetries, lastError)
}

func (client *Client) callZigLLM(_ context.Context, systemInstruction, userPrompt string, data []byte, mimeType string) (string, error) {
	dataPointer := unsafe.Pointer(nil)
	if len(data) > 0 {
		dataPointer = unsafe.Pointer(&data[0])
	}
	mimeTypePointer := unsafe.Pointer(unsafe.StringData(mimeType))
	systemInstructionPointer := unsafe.Pointer(unsafe.StringData(systemInstruction))
	userPromptPointer := unsafe.Pointer(unsafe.StringData(userPrompt))

	resultPointer := C.zig_llm_process_image(
		client.handle,
		dataPointer, C.uint32_t(len(data)),
		mimeTypePointer, C.uint32_t(len(mimeType)),
		systemInstructionPointer, C.uint32_t(len(systemInstruction)),
		userPromptPointer, C.uint32_t(len(userPrompt)),
		nil, 0, // No response mime type
		nil, 0, // No response schema
	)

	if resultPointer == nil {
		return "", errors.New("high-integrity orchestration failed in Zig")
	}
	defer C.zig_llm_free_string(resultPointer)

	return C.GoString(resultPointer), nil
}
