package crossover_blacklist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Error struct {
	msg  string
	code int
}

// Config holds configuration to passed to the plugin
type Config struct {
	Blacklist []string
}

// CreateConfig populates the config data object
func CreateConfig() *Config {
	return &Config{}
}

type CrossoverBlacklist struct {
	next      http.Handler
	name      string
	client    http.Client
	blacklist map[string]bool
}

// New created a new  plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if len(config.Blacklist) == 0 {
		return nil, fmt.Errorf("blacklist empty")
	}

	requestHandler := &CrossoverBlacklist{
		next: next,
		name: name,
		client: http.Client{
			Timeout: 5 * time.Second,
		},
		blacklist: make(map[string]bool),
	}
	for _, v := range config.Blacklist {
		requestHandler.blacklist[v] = true
	}

	return requestHandler, nil
}

func (a *CrossoverBlacklist) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	reqClone, err := a.clone(req)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte("can't clone request body"))
		return
	}

	blackListedErr := a.blacklisted(reqClone)
	if blackListedErr != nil {
		rw.WriteHeader(blackListedErr.code)
		rw.Write([]byte(blackListedErr.msg))
		return
	}
	a.next.ServeHTTP(rw, req)
}

// clone returns a deep copy of request
func (a *CrossoverBlacklist) clone(req *http.Request) (clone *http.Request, err error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	clonedRequest := req.Clone(req.Context())
	req.Body = io.NopCloser(bytes.NewReader(body))
	clonedRequest.Body = io.NopCloser(bytes.NewReader(body))
	return clonedRequest, nil
}

func (a *CrossoverBlacklist) blacklisted(req *http.Request) *Error {
	type Request struct {
		Method string `json:"method"`
	}
	var request Request
	bodyBytes, _ := io.ReadAll(req.Body)
	err := json.Unmarshal(bodyBytes, &request)
	// Try to unmarshal as request with single object body
	if err != nil {
		// If failed, try to unmarshal as array of objects body
		var multipleObjects []Request
		err = json.Unmarshal(bodyBytes, &multipleObjects)
		if err != nil {
			// If both attempts failed, it's not a valid JSON request
			return &Error{
				msg:  err.Error(),
				code: http.StatusBadRequest,
			}
		}
		//validate array of requests
		for _, object := range multipleObjects {
			_, ok := a.blacklist[strings.ToLower(strings.TrimSpace(object.Method))]
			if ok {
				return &Error{
					msg:  fmt.Sprintf("method %s is not allowed", object.Method),
					code: http.StatusMethodNotAllowed,
				}
			}

		}
		return nil
	}

	_, ok := a.blacklist[strings.ToLower(strings.TrimSpace(request.Method))]
	if ok {
		return &Error{
			msg:  "method is not allowed",
			code: http.StatusMethodNotAllowed,
		}
	}

	return nil
}
