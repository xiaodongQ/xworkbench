package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/xiaodongQ/xworkbench/internal/backend"
    "go.uber.org/zap"
)

func TestDebugExport(t *testing.T) {
    db, _, err := backend.TestDB()
    if err != nil {
        t.Fatal(err)
    }
    if logger == nil {
        z, _ := zap.NewProduction()
        logger = z.Sugar()
    }
    s := &APIServer{
        db:      backend.NewTaskRepo(db),
        expDB:   backend.NewExperienceRepo(db),
        dirDB:   backend.NewDirShortcutRepo(db),
        linkDB:  backend.NewWebLinkRepo(db),
        schedDB: backend.NewScheduledTaskRepo(db),
        running: map[string]context.CancelFunc{},
    }
    mux := http.NewServeMux()
    mux.HandleFunc("GET /api/config/export", s.handleConfigExport)
    mux.HandleFunc("POST /api/config/import/preview", s.handleConfigImportPreview)
    mux.HandleFunc("POST /api/config/import", s.handleConfigImport)

    _ = s.expDB.Create(&backend.Experience{
        ID: "exp-long-1", Module: "git", Keywords: "rebase,merge", Scene: "feature 收尾",
        Details: "# Title\n\n- step 1\n- step 2\n\n```bash\necho hello\n```\n\nEnd.", Version: "v1.0.0",
    })

    mux2 := http.NewServeMux()
    mux2.HandleFunc("GET /api/config/export", s.handleConfigExport)
    mux2.HandleFunc("POST /api/config/import/preview", s.handleConfigImportPreview)
    mux2.HandleFunc("POST /api/config/import", s.handleConfigImport)

    req := httptest.NewRequest("GET", "/api/config/export?types=experiences", nil)
    w := httptest.NewRecorder()
    mux2.ServeHTTP(w, req)
    fmt.Printf("Status: %d\n", w.Code)
    fmt.Printf("Body:\n%s\n", w.Body.String())

    var got map[string]json.RawMessage
    if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
        fmt.Printf("Unmarshal error: %v\n", err)
        return
    }
    fmt.Printf("Keys: %v\n", func() []string {
        ks := make([]string, 0, len(got))
        for k := range got {
            ks = append(ks, k)
        }
        return ks
    }())
}
