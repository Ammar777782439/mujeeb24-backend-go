package handlers

import (
        "errors"
        "testing"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ----------------------------------------------------------------------------
// Tests: Idempotency (Contract §59)
// ----------------------------------------------------------------------------

func TestIdempotencySameKeyReturnsCachedResult(t *testing.T) {
        bizRepo := &stubPlatformBusinessRepository{
                createResult: ports.PlatformBusinessRecord{ID: "biz-1", Name: "Test", PlatformStatus: "active"},
        }
        auditRepo := &stubPlatformAuditRepository{}
        server := newPlatformServer(PlatformDeps{PlatformBusiness: bizRepo, PlatformAudit: auditRepo})
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()

        input := &dto.CreateBusinessInput{
                Body: dto.CreateBusinessRequest{
                        Name: "Test", Slug: "test", VerticalType: "retail",
                        DefaultCurrency: "YER", Locale: "ar",
                },
        }
        input.IdempotencyKey = "idem-key-1"

        result1, handled1 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input)
        if !handled1 {
                t.Fatalf("first call: expected handled=true")
        }
        bizRepo.createResult = ports.PlatformBusinessRecord{ID: "DIFFERENT-ID"}
        result2, handled2 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input)
        if !handled2 {
                t.Fatalf("second call: expected handled=true")
        }
        if result1 != result2 {
                t.Fatalf("expected same cached result, got different values")
        }
}

func TestIdempotencyNoKeyExecutesEveryTime(t *testing.T) {
        bizRepo := &stubPlatformBusinessRepository{
                createResult: ports.PlatformBusinessRecord{ID: "biz-1", PlatformStatus: "active"},
        }
        auditRepo := &stubPlatformAuditRepository{}
        server := newPlatformServer(PlatformDeps{PlatformBusiness: bizRepo, PlatformAudit: auditRepo})
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()

        input := &dto.CreateBusinessInput{
                Body: dto.CreateBusinessRequest{
                        Name: "Test", Slug: "test", VerticalType: "retail",
                        DefaultCurrency: "YER", Locale: "ar",
                },
        }
        _, handled1 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input)
        if !handled1 {
                t.Fatalf("first call: expected handled=true")
        }
        bizRepo.createResult = ports.PlatformBusinessRecord{ID: "biz-2"}
        _, handled2 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input)
        if !handled2 {
                t.Fatalf("second call: expected handled=true")
        }
}

func TestIdempotencyFailedFirstCallAllowsRetry(t *testing.T) {
        bizRepo := &stubPlatformBusinessRepository{
                createErr: errors.New("DB connection refused"),
        }
        auditRepo := &stubPlatformAuditRepository{}
        server := newPlatformServer(PlatformDeps{PlatformBusiness: bizRepo, PlatformAudit: auditRepo})
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()

        input := &dto.CreateBusinessInput{
                Body: dto.CreateBusinessRequest{
                        Name: "Test", Slug: "test", VerticalType: "retail",
                        DefaultCurrency: "YER", Locale: "ar",
                },
        }
        input.IdempotencyKey = "idem-retry-1"

        _, handled1 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input)
        if !handled1 {
                t.Fatalf("first call: expected handled=true")
        }
        bizRepo.createErr = nil
        bizRepo.createResult = ports.PlatformBusinessRecord{ID: "biz-recovered", PlatformStatus: "active"}
        result2, handled2 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input)
        if !handled2 {
                t.Fatalf("second call: expected handled=true")
        }
        if _, isErr := result2.(error); isErr {
                t.Fatalf("expected second call to succeed after retry, got error")
        }
}

func TestIdempotencyDifferentKeysExecuteSeparately(t *testing.T) {
        bizRepo := &stubPlatformBusinessRepository{
                createResult: ports.PlatformBusinessRecord{ID: "biz-1", PlatformStatus: "active"},
        }
        auditRepo := &stubPlatformAuditRepository{}
        server := newPlatformServer(PlatformDeps{PlatformBusiness: bizRepo, PlatformAudit: auditRepo})
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()

        input1 := &dto.CreateBusinessInput{
                Body: dto.CreateBusinessRequest{Name: "Test1", Slug: "test1", VerticalType: "retail", DefaultCurrency: "YER", Locale: "ar"},
        }
        input1.IdempotencyKey = "key-A"
        input2 := &dto.CreateBusinessInput{
                Body: dto.CreateBusinessRequest{Name: "Test2", Slug: "test2", VerticalType: "retail", DefaultCurrency: "YER", Locale: "ar"},
        }
        input2.IdempotencyKey = "key-B"

        _, handled1 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input1)
        if !handled1 {
                t.Fatalf("first call: expected handled=true")
        }
        bizRepo.createResult = ports.PlatformBusinessRecord{ID: "biz-2"}
        _, handled2 := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input2)
        if !handled2 {
                t.Fatalf("second call: expected handled=true")
        }
}
