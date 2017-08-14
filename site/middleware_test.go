package site_test

import (
	"net/http"
	"testing"

	"github.com/Prutswonder/go-servicefoundation/model"
	"github.com/Prutswonder/go-servicefoundation/site"
	. "github.com/Prutswonder/go-servicefoundation/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMiddlewareWrapperImpl_Wrap(t *testing.T) {
	scenarios := []model.Middleware{model.CORS, model.NoCaching, model.Counter, model.Histogram}

	for i, scenario := range scenarios {
		const subSystem = "my-sub"
		const name = "my-name"
		log := &MockLogger{}
		m := &MockMetrics{}
		corsOptions := &model.CORSOptions{}
		called := false
		handle := func(model.WrappedResponseWriter, *http.Request, model.RouterParams) {
			called = true
		}
		rdr := &MockReader{}
		r, _ := http.NewRequest("GET", "https://www.site.com/some/url", rdr)
		w := &MockResponseWriter{}
		h := &MockMetricsHistogram{}
		p := model.RouterParams{}
		sut := site.CreateMiddlewareWrapper(log, m, corsOptions)

		w.On("Header").Return(http.Header{})
		w.On("Status").Return(http.StatusOK)
		h.On("RecordTimeElapsed", mock.Anything)
		m.On("Count", subSystem, mock.Anything, mock.Anything)
		m.On("CountLabels", subSystem, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		m.On("AddHistogram", subSystem, mock.Anything, mock.Anything).Return(h)

		// Act
		actual := sut.Wrap(subSystem, name, scenario, handle)

		assert.NotNil(t, actual, "Scenario %n", i)
		assert.NotEqual(t, handle, actual, "Scenario %n", i)

		actual(w, r, p)
		assert.True(t, called, "Scenario %n", i)
	}
}

func TestMiddlewareWrapperImpl_Wrap_UnknownMiddleware_ReturnsUnwrappedHandler(t *testing.T) {
	const subSystem = "my-sub"
	const name = "my-name"
	log := &MockLogger{}
	m := &MockMetrics{}
	corsOptions := &model.CORSOptions{}
	handle := func(model.WrappedResponseWriter, *http.Request, model.RouterParams) {
	}
	w := &MockResponseWriter{}
	h := &MockMetricsHistogram{}
	sut := site.CreateMiddlewareWrapper(log, m, corsOptions)

	log.On("Warn", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	w.On("Header").Return(http.Header{})
	w.On("Status").Return(http.StatusOK)
	h.On("RecordTimeElapsed", mock.Anything)
	m.On("Count", subSystem, mock.Anything, mock.Anything)
	m.On("CountLabels", subSystem, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("AddHistogram", subSystem, mock.Anything, mock.Anything).Return(h)

	// Act
	actual := sut.Wrap(subSystem, name, 0, handle)

	assert.NotNil(t, actual)
	log.AssertExpectations(t)
}

func TestMiddlewareWrapperImpl_Wrap_PanicsAreHandled(t *testing.T) {
	scenarios := []model.Middleware{model.Counter, model.Histogram}

	for i, scenario := range scenarios {
		const subSystem = "my-sub"
		const name = "my-name"
		log := &MockLogger{}
		m := &MockMetrics{}
		corsOptions := &model.CORSOptions{}
		handle := func(model.WrappedResponseWriter, *http.Request, model.RouterParams) {
			panic("whoa")
		}
		rdr := &MockReader{}
		r, _ := http.NewRequest("GET", "https://www.site.com/some/url", rdr)
		w := &MockResponseWriter{}
		h := &MockMetricsHistogram{}
		p := model.RouterParams{}
		sut := site.CreateMiddlewareWrapper(log, m, corsOptions)

		log.On("Error", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		w.On("Header").Return(http.Header{})
		w.On("Status").Return(http.StatusOK)
		h.On("RecordTimeElapsed", mock.Anything)
		m.On("Count", subSystem, mock.Anything, mock.Anything)
		m.On("CountLabels", subSystem, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		m.On("AddHistogram", subSystem, mock.Anything, mock.Anything).Return(h)

		// Act
		actual := sut.Wrap(subSystem, name, scenario, handle)

		assert.NotNil(t, actual, "Scenario %n", i)
		assert.NotEqual(t, handle, actual, "Scenario %n", i)

		actual(w, r, p)
		log.AssertExpectations(t)
	}
}
