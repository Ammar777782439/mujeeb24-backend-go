# Mujeeb AI Runtime — provider isolation

Source branch: `feat/gemini-deepseek`; implementation branch: `refactor/provider-agnostic-ai-runtime`.

> **دليل الإضافة الرسمي المفصل (Customer + Catalog + Merchant):** [إضافة نموذج أو مزود AI جديد — خطوات التنفيذ والاختبارات](adding-ai-provider-runbook-ar.md). هذا المستند مرجع الحدود المعمارية السريع؛ يُرجع إلى الدليل المفصل عند إدخال مزود جديد.

## Responsibility boundaries

- The language model chooses among the **capabilities offered by Mujeeb**. No Go intent engine or keyword classifier decides `evaluate_full_catalog`.
- Application services own catalog evidence retrieval, tenant isolation, authorization, validation, transactions, handoff, and human review.
- Gateway adapters produce canonical requests/decisions. Transport provider adapters translate messages, strict tool JSON schemas, tool-call responses and errors to the selected vendor's protocol.
- Bootstrap selects the deployment's supported provider in code, not through a merchant dashboard, and provides all AI use cases with the same canonical completion client.
- Platform Admin reports the actual selected model and credential presence; credentials are not accepted through HTTP.
- Token admission estimates are conservative if the vendor has no verified token counting endpoint; incomplete catalog evidence is never silently truncated.
- No provider-specific raw response is written into ordinary logs. Explicit diagnostic catalog dumps remain independently controlled and may contain sensitive information.

## DeepSeek deployment

Set `NATIVE_AI_ENABLED=true`, `DEEPSEEK_API_KEY` and an API-supported `DEEPSEEK_MODEL` (default `deepseek-flash`). No Gemini key is needed. The DeepSeek adapter uses documented `/beta/chat/completions` strict tool calling in non-thinking mode. Keep the context/window budget within the verified limits of the deployed model.

`AUTOREPLY_ENABLED` and `CUSTOMER_SALES_ENABLED` retain their existing independent feature gates and SocialAPI prerequisites.

Historical SQL migrations, encrypted records and provider-spend records must remain intact. Never rewrite applied migrations simply to remove a vendor integration.

## Acceptance gates

1. Compile and run full Go tests, including race tests for `native` and `gateway`.
2. Start without `GEMINI_API_KEY`; fail closed if `NATIVE_AI_ENABLED=true` but `DEEPSEEK_API_KEY` is missing.
3. Exercise a customer capability selection, `evaluate_full_catalog` and a terminal decision with the same DeepSeek provider.
4. Test merchant authoring tool rounds, summary generation, strict malformed JSON rejection, tenant checks, and human approval.
5. Verify token counts, cost/pricing state, admin model display, redacted logs, 429/503 handling, and deployment rollback.
6. Qualify the configured DeepSeek model using real account requests before deploying to production. Mocked HTTP tests do not establish live compatibility.

Production deployment is NOT authorized by the existence of this branch or a green build.
