package testutils

// import (
//     "encoding/json"
//     "net/http"
//     "net/http/httptest"
//     "testing"

//     "github.com/stretchr/testify/require"
// )

// // HTTPTestServer creates a test HTTP server with the given handler
// func HTTPTestServer(t *testing.T, handler http.Handler) (*http.ServeMux, string, func()) {
//     t.Helper()
    
//     api := http.NewServeMux()
//     api.Handle("/", handler)
//     srv := httptest.NewServer(api)
    
//     return api, srv.URL, srv.Close
// }

// // ReadJSON reads and unmarshals JSON from the response
// func ReadJSON(t *testing.T, resp *http.Response, target interface{}) {
//     t.Helper()
//     defer resp.Body.Close()
//     require.NoError(t, json.NewDecoder(resp.Body).Decode(target))
// }

// // MustMarshal marshals the given value to JSON, failing the test on error
// func MustMarshal(t *testing.T, v interface{}) []byte {
//     t.Helper()
//     data, err := json.Marshal(v)
//     require.NoError(t, err)
//     return data
// }