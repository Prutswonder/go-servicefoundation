package servicefoundation_test

import (
	"testing"

	sf "github.com/Prutswonder/go-servicefoundation"
	"github.com/stretchr/testify/assert"
)

func TestNewRouterFactory(t *testing.T) {
	sut := sf.NewRouterFactory()

	assert.NotNil(t, sut)

	actual := sut.NewRouter()

	assert.NotNil(t, actual)
}
