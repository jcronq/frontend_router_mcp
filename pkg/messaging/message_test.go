package messaging

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResponseSender implements the ResponseSender interface for testing
type mockResponseSender struct {
	messages []*Message
	mu       sync.Mutex
	err      error
}

func (m *mockResponseSender) SendResponse(msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.err != nil {
		return m.err
	}
	
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockResponseSender) GetMessages() []*Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	msgs := make([]*Message, len(m.messages))
	copy(msgs, m.messages)
	return msgs
}

func (m *mockResponseSender) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// TestNewMessage tests the NewMessage function
func TestNewMessage(t *testing.T) {
	tests := []struct {
		name        string
		messageType string
		payload     interface{}
		wantErr     bool
	}{
		{
			name:        "string payload",
			messageType: "test",
			payload:     "hello world",
			wantErr:     false,
		},
		{
			name:        "map payload",
			messageType: "user_response",
			payload:     map[string]interface{}{"answer": "yes", "id": 123},
			wantErr:     false,
		},
		{
			name:        "struct payload",
			messageType: "connect",
			payload:     struct{ Status string }{"connected"},
			wantErr:     false,
		},
		{
			name:        "nil payload",
			messageType: "ping",
			payload:     nil,
			wantErr:     false,
		},
		{
			name:        "empty type",
			messageType: "",
			payload:     "test",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := NewMessage(tt.messageType, tt.payload)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, msg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, msg)
				assert.Equal(t, tt.messageType, msg.Type)
				assert.NotEmpty(t, msg.ID)
				assert.False(t, msg.Timestamp.IsZero())
				assert.NotNil(t, msg.Payload)
				
				// Verify payload can be unmarshaled back
				var unmarshaled interface{}
				err = json.Unmarshal(msg.Payload, &unmarshaled)
				assert.NoError(t, err)
			}
		})
	}
}

// TestNewMessage_InvalidPayload tests NewMessage with invalid payload
func TestNewMessage_InvalidPayload(t *testing.T) {
	// Create a payload that cannot be marshaled to JSON
	invalidPayload := make(chan int) // channels cannot be marshaled
	
	msg, err := NewMessage("test", invalidPayload)
	
	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "json: unsupported type")
}

// TestMessage_ParsePayload tests the ParsePayload method
func TestMessage_ParsePayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  interface{}
		target   interface{}
		wantErr  bool
		validate func(t *testing.T, target interface{})
	}{
		{
			name:    "string to string",
			payload: "hello world",
			target:  new(string),
			wantErr: false,
			validate: func(t *testing.T, target interface{}) {
				str := target.(*string)
				assert.Equal(t, "hello world", *str)
			},
		},
		{
			name:    "map to map",
			payload: map[string]interface{}{"key": "value", "num": 42},
			target:  new(map[string]interface{}),
			wantErr: false,
			validate: func(t *testing.T, target interface{}) {
				m := target.(*map[string]interface{})
				assert.Equal(t, "value", (*m)["key"])
				assert.Equal(t, float64(42), (*m)["num"]) // JSON unmarshals numbers as float64
			},
		},
		{
			name:    "struct to struct",
			payload: struct{ Name string; Age int }{"John", 30},
			target:  new(struct{ Name string; Age int }),
			wantErr: false,
			validate: func(t *testing.T, target interface{}) {
				s := target.(*struct{ Name string; Age int })
				assert.Equal(t, "John", s.Name)
				assert.Equal(t, 30, s.Age)
			},
		},
		{
			name:    "invalid target type",
			payload: "hello",
			target:  new(int),
			wantErr: true,
			validate: func(t *testing.T, target interface{}) {
				// Should not be called for error cases
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := NewMessage("test", tt.payload)
			require.NoError(t, err)
			
			err = msg.ParsePayload(tt.target)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				tt.validate(t, tt.target)
			}
		})
	}
}

// TestMessage_ParsePayload_InvalidJSON tests ParsePayload with invalid JSON
func TestMessage_ParsePayload_InvalidJSON(t *testing.T) {
	msg := &Message{
		ID:        "test-id",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{"invalid": json}`), // Invalid JSON
	}
	
	var target map[string]interface{}
	err := msg.ParsePayload(&target)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
}

// TestGenerateID tests the GenerateID function
func TestGenerateID(t *testing.T) {
	// Test that IDs are generated
	id1 := GenerateID()
	id2 := GenerateID()
	
	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
	
	// Test UUID format (36 characters with dashes)
	assert.Len(t, id1, 36)
	assert.Contains(t, id1, "-")
	
	// Test uniqueness over multiple generations
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := GenerateID()
		assert.False(t, ids[id], "Generated duplicate ID: %s", id)
		ids[id] = true
	}
}

// TestRandString tests the RandString function
func TestRandString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"zero length", 0},
		{"single character", 1},
		{"normal length", 8},
		{"long string", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RandString(tt.length)
			assert.Len(t, result, tt.length)
			
			if tt.length > 0 {
				// Check that all characters are from the expected set
				const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
				for _, char := range result {
					assert.Contains(t, letters, string(char))
				}
			}
		})
	}
}

// TestRandString_Uniqueness tests that RandString generates unique strings
func TestRandString_Uniqueness(t *testing.T) {
	const length = 8
	const numStrings = 1000
	
	strings := make(map[string]bool)
	for i := 0; i < numStrings; i++ {
		str := RandString(length)
		assert.False(t, strings[str], "Generated duplicate string: %s", str)
		strings[str] = true
	}
}

// TestMessage_Serialization tests message serialization and deserialization
func TestMessage_Serialization(t *testing.T) {
	original := &Message{
		ID:        "test-id-123",
		Type:      "user_response",
		Timestamp: time.Now().UTC().Truncate(time.Second), // Truncate for comparison
		Payload:   json.RawMessage(`{"answer": "yes", "id": 42}`),
		ClientID:  "client-123",
	}
	
	// Serialize to JSON
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)
	
	// Deserialize from JSON
	var deserialized Message
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err)
	
	// Compare fields (ResponseSender should be nil after deserialization)
	assert.Equal(t, original.ID, deserialized.ID)
	assert.Equal(t, original.Type, deserialized.Type)
	assert.Equal(t, original.Timestamp, deserialized.Timestamp)
	assert.Equal(t, original.Payload, deserialized.Payload)
	assert.Equal(t, original.ClientID, deserialized.ClientID)
	assert.Nil(t, deserialized.ResponseSender)
}

// TestMessage_ResponseSender tests ResponseSender functionality
func TestMessage_ResponseSender(t *testing.T) {
	sender := &mockResponseSender{}
	
	msg := &Message{
		ID:             "test-id",
		Type:           "test",
		Timestamp:      time.Now(),
		Payload:        json.RawMessage(`"test"`),
		ResponseSender: sender,
	}
	
	// Test that ResponseSender is set
	assert.NotNil(t, msg.ResponseSender)
	assert.Equal(t, sender, msg.ResponseSender)
	
	// Test sending a response
	response := &Message{
		ID:        "response-id",
		Type:      "response",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"response"`),
	}
	
	err := msg.ResponseSender.SendResponse(response)
	assert.NoError(t, err)
	
	messages := sender.GetMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, response, messages[0])
}

// TestMessage_ResponseSender_Error tests ResponseSender error handling
func TestMessage_ResponseSender_Error(t *testing.T) {
	sender := &mockResponseSender{}
	sender.SetError(fmt.Errorf("send error"))
	
	msg := &Message{
		ID:             "test-id",
		Type:           "test",
		Timestamp:      time.Now(),
		Payload:        json.RawMessage(`"test"`),
		ResponseSender: sender,
	}
	
	response := &Message{
		ID:        "response-id",
		Type:      "response",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"response"`),
	}
	
	err := msg.ResponseSender.SendResponse(response)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "send error")
}

// TestMessage_ConcurrentAccess tests concurrent access to message fields
func TestMessage_ConcurrentAccess(t *testing.T) {
	msg := &Message{
		ID:        "test-id",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test"`),
		ClientID:  "client-123",
	}
	
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	// Test concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			
			// Read all fields
			_ = msg.ID
			_ = msg.Type
			_ = msg.Timestamp
			_ = msg.Payload
			_ = msg.ClientID
			_ = msg.ResponseSender
		}()
	}
	
	wg.Wait()
	
	// Verify message is still intact
	assert.Equal(t, "test-id", msg.ID)
	assert.Equal(t, "test", msg.Type)
	assert.Equal(t, "client-123", msg.ClientID)
}

// TestMessage_EdgeCases tests edge cases and boundary conditions
func TestMessage_EdgeCases(t *testing.T) {
	t.Run("very long strings", func(t *testing.T) {
		longString := strings.Repeat("a", 10000)
		msg, err := NewMessage("test", longString)
		
		assert.NoError(t, err)
		assert.NotNil(t, msg)
		
		var result string
		err = msg.ParsePayload(&result)
		assert.NoError(t, err)
		assert.Equal(t, longString, result)
	})
	
	t.Run("empty payload", func(t *testing.T) {
		msg, err := NewMessage("test", "")
		
		assert.NoError(t, err)
		assert.NotNil(t, msg)
		
		var result string
		err = msg.ParsePayload(&result)
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})
	
	t.Run("complex nested structure", func(t *testing.T) {
		complex := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": []interface{}{
						"item1", 
						2, 
						map[string]interface{}{"nested": true},
					},
				},
			},
		}
		
		msg, err := NewMessage("complex", complex)
		assert.NoError(t, err)
		
		var result map[string]interface{}
		err = msg.ParsePayload(&result)
		assert.NoError(t, err)
		
		// Verify complex structure is preserved
		level1 := result["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})
		level3 := level2["level3"].([]interface{})
		
		assert.Equal(t, "item1", level3[0])
		assert.Equal(t, float64(2), level3[1])
		nested := level3[2].(map[string]interface{})
		assert.Equal(t, true, nested["nested"])
	})
}

// BenchmarkNewMessage benchmarks the NewMessage function
func BenchmarkNewMessage(b *testing.B) {
	payload := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": []string{"a", "b", "c"},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewMessage("test", payload)
	}
}

// BenchmarkMessage_ParsePayload benchmarks the ParsePayload method
func BenchmarkMessage_ParsePayload(b *testing.B) {
	payload := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": []string{"a", "b", "c"},
	}
	
	msg, _ := NewMessage("test", payload)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result map[string]interface{}
		_ = msg.ParsePayload(&result)
	}
}

// BenchmarkGenerateID benchmarks the GenerateID function
func BenchmarkGenerateID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateID()
	}
}

// BenchmarkRandString benchmarks the RandString function
func BenchmarkRandString(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RandString(8)
	}
}

// TestMessage_JSONMarshaling tests JSON marshaling behavior
func TestMessage_JSONMarshaling(t *testing.T) {
	msg := &Message{
		ID:        "test-id",
		Type:      "test",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Payload:   json.RawMessage(`{"key": "value"}`),
		ClientID:  "client-123",
	}
	
	// Marshal to JSON
	jsonData, err := json.Marshal(msg)
	require.NoError(t, err)
	
	// Verify JSON contains expected fields
	var jsonMap map[string]interface{}
	err = json.Unmarshal(jsonData, &jsonMap)
	require.NoError(t, err)
	
	assert.Equal(t, "test-id", jsonMap["id"])
	assert.Equal(t, "test", jsonMap["type"])
	assert.Equal(t, "client-123", jsonMap["client_id"])
	assert.Contains(t, jsonMap, "timestamp")
	assert.Contains(t, jsonMap, "payload")
	
	// Verify ResponseSender is not included (marked with json:"-")
	assert.NotContains(t, jsonMap, "ResponseSender")
	assert.NotContains(t, jsonMap, "response_sender")
}

// TestRandString_DeterministicBehavior tests edge cases in RandString
func TestRandString_DeterministicBehavior(t *testing.T) {
	t.Run("negative length", func(t *testing.T) {
		result := RandString(-1)
		assert.Empty(t, result)
	})
	
	t.Run("zero length", func(t *testing.T) {
		result := RandString(0)
		assert.Empty(t, result)
	})
	
	t.Run("large length", func(t *testing.T) {
		result := RandString(10000)
		assert.Len(t, result, 10000)
	})
}