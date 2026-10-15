//go:build ignore

// Read-only diagnostic: reproduce the production Go net/http request fingerprint.
// Run with "go run scripts/litellm-go-edge-probe/main.go". No model inference or logging secrets.
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"
)

const base = "https://webhook.mujeeb24.com"

type safeResponse struct {
    Label string `json:"label"`
    HTTPStatus int `json:"http_status"`
    Server string `json:"server"`
    CFRayPresent bool `json:"cf_ray_present"`
    CFError bool `json:"cloudflare_error"`
    CFCode string `json:"cloudflare_code,omitempty"`
    CFCategory string `json:"cloudflare_category,omitempty"`
}

func safeToken(v interface{}) string {
    s, ok := v.(string)
    if !ok || len(s) > 40 || len(s) == 0 { return "" }
    for _, r := range s {
        if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
            (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') { return "" }
    }
    return s
}

func probe(client *http.Client, endpoint, label, key string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()
    req, err := http.NewRequestWithContext(ctx, "GET", base+endpoint, nil)
    if err != nil { return err }
    // LiteLLM Go model discovery uses Accept + Authorization and no custom UA.
    req.Header.Set("Accept", "application/json")
    if key != "" { req.Header.Set("Authorization", "Bearer "+key) }
    resp, err := client.Do(req)
    if err != nil { return fmt.Errorf("%s network error of type %T",label,err) }
    defer resp.Body.Close()
    result := safeResponse{Label:label,HTTPStatus:resp.StatusCode,Server:resp.Header.Get("Server"),CFRayPresent:resp.Header.Get("Cf-Ray")!=""}
    if resp.StatusCode >= 400 {
        raw, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
        if err == nil {
            var obj map[string]interface{}
            if json.Unmarshal(raw, &obj) == nil {
                flag, _ := obj["cloudflare_error"].(bool)
                result.CFError=flag
                result.CFCode=safeToken(obj["error_code"])
                result.CFCategory=safeToken(obj["error_category"])
            }
        }
    }
    // Print ONLY known response codes and strictly sanitized metadata.
    if result.Server != "cloudflare" && result.Server != "" { result.Server="other" }
    data, _ := json.Marshal(result)
    fmt.Println(string(data))
    return nil
}
func main() {
    key := os.Getenv("LITELLM_API_KEY")
    if key == "" {
        fmt.Fprintln(os.Stderr,"LITELLM_API_KEY environment secret not set")
        os.Exit(1)
    }
    client:=&http.Client{Timeout:20*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
    if err:=probe(client,"/health/readiness","go_readiness","");err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
    if err:=probe(client,"/v1/models","go_anonymous_models","");err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
    if err:=probe(client,"/v1/models","go_authenticated_models",key);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
