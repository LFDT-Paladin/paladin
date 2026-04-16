package identityresolver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	pbIdentityResolver "github.com/LFDT-Paladin/paladin/core/pkg/proto/identityresolver"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/cache"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestResolveVerifier(t *testing.T) {
	r := &identityResolver{}
	_, err := r.ResolveVerifier(context.Background(), "something$bad", "bad algorithm", "bad type")
	assert.ErrorContains(t, err, "PD020006: Locator string something$bad is invalid")

	capacity := 100
	config := &pldconf.CacheConfig{Capacity: &capacity}
	r = &identityResolver{
		nodeName:      "testnode",
		verifierCache: cache.NewCache[string, string](config, config),
		keyManager:    componentsmocks.NewKeyManager(t),
	}
	waitChan := make(chan time.Time)
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier: "0x1234567890abcdef",
			Type:     "ETH_ADDRESS",
		},
	}
	r.keyManager.(*componentsmocks.KeyManager).On("ResolveKeyNewDatabaseTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).WaitUntil(waitChan).Return(resolvedKey, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer func() {
		cancel()
		close(waitChan)
	}()

	done := make(chan bool)
	go func() {
		_, err := r.ResolveVerifier(ctx, "something@testnode", algorithms.Curve_SECP256K1, verifiers.ETH_ADDRESS)
		assert.ErrorContains(t, err, "PD010301: Context canceled")
		done <- true
	}()
	<-done
}

func TestResolveVerifierAsync(t *testing.T) {
	r := &identityResolver{}
	resolved := func(ctx context.Context, verifier string) {
		t.Logf("Resolved verifier: %s", verifier)
	}
	errhandler := func(ctx context.Context, err error) {
		assert.ErrorContains(t, err, "PD020006: Locator string something$bad is invalid")
	}
	r.ResolveVerifierAsync(context.Background(), "something$bad", "bad algorithm", "bad type", resolved, errhandler)
}

func TestNewIdentityResolver(t *testing.T) {
	capacity := 100
	ctx := context.Background()
	conf := &pldconf.IdentityResolverConfig{
		VerifierCache: pldconf.CacheConfig{
			Capacity: &capacity,
		},
	}

	ir := NewIdentityResolver(ctx, conf)

	assert.NotNil(t, ir)
	assert.NotNil(t, ir.(*identityResolver).inflightRequests)
	assert.NotNil(t, ir.(*identityResolver).inflightRequestsMutex)
}
func TestCacheKey(t *testing.T) {
	tests := []struct {
		identifier   string
		node         string
		algorithm    string
		verifierType string
		expected     string
	}{
		{
			identifier:   "id1",
			node:         "node1",
			algorithm:    "alg1",
			verifierType: "type1",
			expected:     "id1@node1|alg1|type1",
		},
		{
			identifier:   "id2",
			node:         "node2",
			algorithm:    "alg2",
			verifierType: "type2",
			expected:     "id2@node2|alg2|type2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := cacheKey(tt.identifier, tt.node, tt.algorithm, tt.verifierType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandlePaladinMsg_ResolveVerifierRequest(t *testing.T) {
	ctx := context.Background()
	capacity := 100
	config := &pldconf.CacheConfig{Capacity: &capacity}

	mockKeyManager := componentsmocks.NewKeyManager(t)
	mockTransportManager := componentsmocks.NewTransportManager(t)

	ir := &identityResolver{
		bgCtx:                 ctx,
		nodeName:              "testnode",
		verifierCache:         cache.NewCache[string, string](config, config),
		keyManager:            mockKeyManager,
		transportManager:      mockTransportManager,
		inflightRequests:      make(map[string]*inflightRequest),
		inflightRequestsMutex: &sync.Mutex{},
	}

	// Create a valid ResolveVerifierRequest message
	request := &pbIdentityResolver.ResolveVerifierRequest{
		Lookup:       "test@testnode",
		Algorithm:    algorithms.Curve_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
	}
	payload, err := proto.Marshal(request)
	require.NoError(t, err)

	messageID := uuid.New()
	fromNode := "remotenode"

	// Mock the key manager to return a resolved key
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier: "0x1234567890abcdef",
			Type:     "ETH_ADDRESS",
		},
	}
	mockKeyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(resolvedKey, nil)

	// Mock the transport manager to send a response
	mockTransportManager.On("Send", mock.Anything, mock.MatchedBy(func(msg *components.FireAndForgetMessageSend) bool {
		return msg.MessageType == "ResolveVerifierResponse" &&
			msg.Node == fromNode &&
			msg.CorrelationID != nil &&
			*msg.CorrelationID == messageID
	})).Return(nil)

	message := &components.ReceivedMessage{
		FromNode:    fromNode,
		MessageID:   messageID,
		MessageType: "ResolveVerifierRequest",
		Payload:     payload,
	}

	// Call HandlePaladinMsg - it spawns a goroutine, so we need to wait a bit
	ir.HandlePaladinMsg(ctx, message)

	// Give the goroutine time to execute
	time.Sleep(100 * time.Millisecond)

	// Verify mocks were called
	mockKeyManager.AssertExpectations(t)
	mockTransportManager.AssertExpectations(t)
}

func TestHandlePaladinMsg_ResolveVerifierResponse(t *testing.T) {
	ctx := context.Background()
	capacity := 100
	config := &pldconf.CacheConfig{Capacity: &capacity}

	ir := &identityResolver{
		bgCtx:                 ctx,
		verifierCache:         cache.NewCache[string, string](config, config),
		inflightRequests:      make(map[string]*inflightRequest),
		inflightRequestsMutex: &sync.Mutex{},
	}

	// Create a valid ResolveVerifierResponse message
	response := &pbIdentityResolver.ResolveVerifierResponse{
		Lookup:       "test@testnode",
		Algorithm:    algorithms.Curve_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     "0x1234567890abcdef",
	}
	payload, err := proto.Marshal(response)
	require.NoError(t, err)

	correlationID := uuid.New()
	correlationIDStr := correlationID.String()

	// Set up an inflight request to verify it gets resolved
	resolvedChan := make(chan string, 1)
	errChan := make(chan error, 1)

	ir.inflightRequestsMutex.Lock()
	ir.inflightRequests[correlationIDStr] = &inflightRequest{
		resolved: func(ctx context.Context, verifier string) {
			resolvedChan <- verifier
		},
		failed: func(ctx context.Context, err error) {
			errChan <- err
		},
	}
	ir.inflightRequestsMutex.Unlock()

	message := &components.ReceivedMessage{
		FromNode:      "remotenode",
		MessageID:     uuid.New(),
		CorrelationID: &correlationID,
		MessageType:   "ResolveVerifierResponse",
		Payload:       payload,
	}

	// Call HandlePaladinMsg - it spawns a goroutine
	ir.HandlePaladinMsg(ctx, message)

	// Wait for the handler to complete and resolve the inflight request
	select {
	case verifier := <-resolvedChan:
		assert.Equal(t, "0x1234567890abcdef", verifier)
	case err := <-errChan:
		t.Fatalf("Unexpected error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for handler to complete")
	}

	// Verify the inflight request was removed
	ir.inflightRequestsMutex.Lock()
	_, exists := ir.inflightRequests[correlationIDStr]
	ir.inflightRequestsMutex.Unlock()
	assert.False(t, exists, "Inflight request should be removed after resolution")
}

func TestHandlePaladinMsg_ResolveVerifierError(t *testing.T) {
	ctx := context.Background()
	capacity := 100
	config := &pldconf.CacheConfig{Capacity: &capacity}

	ir := &identityResolver{
		bgCtx:                 ctx,
		verifierCache:         cache.NewCache[string, string](config, config),
		inflightRequests:      make(map[string]*inflightRequest),
		inflightRequestsMutex: &sync.Mutex{},
	}

	// Create a valid ResolveVerifierError message
	errorMsg := &pbIdentityResolver.ResolveVerifierError{
		Lookup:       "test@testnode",
		Algorithm:    algorithms.Curve_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		ErrorMessage: "Failed to resolve verifier",
	}
	payload, err := proto.Marshal(errorMsg)
	require.NoError(t, err)

	correlationID := uuid.New()
	correlationIDStr := correlationID.String()

	// Set up an inflight request to verify it gets failed
	resolvedChan := make(chan string, 1)
	errChan := make(chan error, 1)

	ir.inflightRequestsMutex.Lock()
	ir.inflightRequests[correlationIDStr] = &inflightRequest{
		resolved: func(ctx context.Context, verifier string) {
			resolvedChan <- verifier
		},
		failed: func(ctx context.Context, err error) {
			errChan <- err
		},
	}
	ir.inflightRequestsMutex.Unlock()

	message := &components.ReceivedMessage{
		FromNode:      "remotenode",
		MessageID:     uuid.New(),
		CorrelationID: &correlationID,
		MessageType:   "ResolveVerifierError",
		Payload:       payload,
	}

	// Call HandlePaladinMsg - it spawns a goroutine
	ir.HandlePaladinMsg(ctx, message)

	// Wait for the handler to complete and fail the inflight request
	select {
	case err := <-errChan:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed to resolve verifier")
	case verifier := <-resolvedChan:
		t.Fatalf("Unexpected resolution: %s", verifier)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for handler to complete")
	}

	// Verify the inflight request was removed
	ir.inflightRequestsMutex.Lock()
	_, exists := ir.inflightRequests[correlationIDStr]
	ir.inflightRequestsMutex.Unlock()
	assert.False(t, exists, "Inflight request should be removed after error")
}

func TestHandlePaladinMsg_UnknownMessageType(t *testing.T) {
	ctx := context.Background()
	capacity := 100
	config := &pldconf.CacheConfig{Capacity: &capacity}

	ir := &identityResolver{
		bgCtx:                 ctx,
		verifierCache:         cache.NewCache[string, string](config, config),
		inflightRequests:      make(map[string]*inflightRequest),
		inflightRequestsMutex: &sync.Mutex{},
	}

	message := &components.ReceivedMessage{
		FromNode:    "remotenode",
		MessageID:   uuid.New(),
		MessageType: "UnknownMessageType",
		Payload:     []byte("some payload"),
	}

	// Call HandlePaladinMsg - it should log an error but not panic
	ir.HandlePaladinMsg(ctx, message)
}

func TestHandlePaladinMsg_ResolveVerifierResponse_InvalidPayload(t *testing.T) {
	ctx := context.Background()
	capacity := 100
	config := &pldconf.CacheConfig{Capacity: &capacity}

	ir := &identityResolver{
		bgCtx:                 ctx,
		verifierCache:         cache.NewCache[string, string](config, config),
		inflightRequests:      make(map[string]*inflightRequest),
		inflightRequestsMutex: &sync.Mutex{},
	}

	correlationID := uuid.New()

	// Use invalid payload that can't be unmarshaled
	message := &components.ReceivedMessage{
		FromNode:      "remotenode",
		MessageID:     uuid.New(),
		CorrelationID: &correlationID,
		MessageType:   "ResolveVerifierResponse",
		Payload:       []byte("invalid proto data"),
	}

	// Call HandlePaladinMsg - it should handle the error gracefully
	ir.HandlePaladinMsg(ctx, message)
}
