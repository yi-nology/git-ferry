package response

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func TestSuccess(t *testing.T) {
	c := app.NewContext(1)
	data := map[string]string{"key": "value"}

	Success(c, data)

	assert.DeepEqual(t, consts.StatusOK, c.Response.StatusCode())

	var resp Response
	err := json.Unmarshal(c.Response.Body(), &resp)
	assert.Nil(t, err)
	assert.DeepEqual(t, consts.StatusOK, resp.Code)
	assert.DeepEqual(t, "success", resp.Message)
	assert.NotNil(t, resp.Data)
	assert.NotNil(t, resp.Timestamp)
}

func TestCreated(t *testing.T) {
	c := app.NewContext(1)
	data := map[string]string{"id": "123"}

	Created(c, data)

	assert.DeepEqual(t, consts.StatusCreated, c.Response.StatusCode())

	var resp Response
	err := json.Unmarshal(c.Response.Body(), &resp)
	assert.Nil(t, err)
	assert.DeepEqual(t, consts.StatusCreated, resp.Code)
	assert.DeepEqual(t, "created", resp.Message)
}

func TestBadRequest(t *testing.T) {
	c := app.NewContext(1)

	BadRequest(c, "invalid input")

	assert.DeepEqual(t, consts.StatusBadRequest, c.Response.StatusCode())

	var resp Response
	err := json.Unmarshal(c.Response.Body(), &resp)
	assert.Nil(t, err)
	assert.DeepEqual(t, consts.StatusBadRequest, resp.Code)
	assert.DeepEqual(t, "invalid input", resp.Message)
}

func TestNotFound(t *testing.T) {
	c := app.NewContext(1)

	NotFound(c, "resource not found")

	assert.DeepEqual(t, consts.StatusNotFound, c.Response.StatusCode())

	var resp Response
	err := json.Unmarshal(c.Response.Body(), &resp)
	assert.Nil(t, err)
	assert.DeepEqual(t, consts.StatusNotFound, resp.Code)
	assert.DeepEqual(t, "resource not found", resp.Message)
}

func TestInternalError(t *testing.T) {
	c := app.NewContext(1)

	InternalError(c, "database connection failed")

	assert.DeepEqual(t, consts.StatusInternalServerError, c.Response.StatusCode())

	var resp Response
	err := json.Unmarshal(c.Response.Body(), &resp)
	assert.Nil(t, err)
	assert.DeepEqual(t, consts.StatusInternalServerError, resp.Code)
	// Internal error should not expose internal details
	assert.DeepEqual(t, "internal server error", resp.Message)
}

func TestNoContent(t *testing.T) {
	c := app.NewContext(1)

	NoContent(c)

	assert.DeepEqual(t, consts.StatusNoContent, c.Response.StatusCode())
}

func TestPaginated(t *testing.T) {
	c := app.NewContext(1)
	list := []string{"a", "b", "c"}

	Paginated(c, list, 10, 1, 3)

	assert.DeepEqual(t, consts.StatusOK, c.Response.StatusCode())

	var resp Response
	err := json.Unmarshal(c.Response.Body(), &resp)
	assert.Nil(t, err)
	assert.DeepEqual(t, consts.StatusOK, resp.Code)
	assert.DeepEqual(t, "success", resp.Message)

	// Check pagination data
	data, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)

	pagination, ok := data["pagination"].(map[string]interface{})
	assert.True(t, ok)
	assert.DeepEqual(t, float64(1), pagination["page"])
	assert.DeepEqual(t, float64(3), pagination["page_size"])
	assert.DeepEqual(t, float64(10), pagination["total"])
	assert.DeepEqual(t, float64(4), pagination["total_pages"])
}

func TestPaginatedEdgeCases(t *testing.T) {
	c := app.NewContext(1)

	// Zero total
	Paginated(c, []string{}, 0, 1, 10)

	var resp Response
	err := json.Unmarshal(c.Response.Body(), &resp)
	assert.Nil(t, err)

	data, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)

	pagination, ok := data["pagination"].(map[string]interface{})
	assert.True(t, ok)
	assert.DeepEqual(t, float64(0), pagination["total_pages"])
}
