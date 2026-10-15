#!/usr/bin/env python3
"""Live LiteLLM model contract smoke, with no mock/provider stub.

Reads a virtual key from the LITELLM_API_KEY environment variable only.
Sends synthetic non-merchant catalog evidence to the real configured provider.
Never prints credentials, raw model replies, or headers.
This is a provider/JSON-contract smoke, NOT an end-to-end merchant DB test.
"""
import json
import os
import sys
import urllib.error
import urllib.request

BASE = os.environ.get("LITELLM_BASE_URL", "").rstrip("/")
KEY = os.environ.get("LITELLM_API_KEY", "")
MODEL = os.environ.get("LITELLM_CATALOG_MODEL", "mujeeb-catalog")
TIMEOUT = 65

def request_json(path, payload=None):
    if not BASE.startswith("https://") or not KEY:
        raise ValueError("A HTTPS LITELLM_BASE_URL and a secret LITELLM_API_KEY are required")
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8") if payload is not None else None
    req = urllib.request.Request(
        BASE + path,
        data=data,
        headers={
            "Authorization": "Bearer " + KEY,
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
        method="POST" if data is not None else "GET",
    )
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as response:
            if response.status != 200:
                raise RuntimeError(f"{path}: unexpected status {response.status}")
            return json.load(response)
    except urllib.error.HTTPError as err:
        # Diagnose the issuing layer without exposing messages, credentials,
        # request headers, response headers or arbitrary provider error text.
        import re
        content_type = (err.headers.get("Content-Type") or "").split(";")[0].lower().strip()
        mit = (err.headers.get("cf-mitigated") or "").lower().strip()
        def safe_identifier(value):
            raw = str(value or "")
            return raw[:80] if re.fullmatch(r"[A-Za-z0-9_.:-]{1,80}", raw) else "unavailable"
        classification = "html_or_gateway" if "html" in content_type else "json_or_api" if "json" in content_type else "unknown"
        error_type = "unavailable"
        error_code = "unavailable"
        if "json" in content_type:
            try:
                payload = json.loads(err.read(4096))
                error = payload.get("error", {}) if isinstance(payload, dict) else {}
                if isinstance(error, dict):
                    error_type = safe_identifier(error.get("type"))
                    error_code = safe_identifier(error.get("code"))
            except (ValueError, UnicodeError):
                pass
        details = f"response_layer={classification} error_type={error_type} error_code={error_code}"
        if mit == "challenge":
            details += " cloudflare_challenge=true"
        raise RuntimeError(f"{path}: HTTP {err.code} {details}") from None
    except urllib.error.URLError as err:
        # Do not dump arbitrary remote error content.
        raise RuntimeError(f"{path}: HTTPS connection failed ({type(err.reason).__name__})") from None

def main():
    try:
        models = request_json("/v1/models")
    except RuntimeError as exc:
        if not str(exc).startswith("/v1/models: HTTP 403 "):
            raise
        # Some LiteLLM virtual keys can invoke an allowed model without
        # permission to enumerate the model registry. This is NOT evidence
        # of a successful model call: still require a real completion.
        print("WARN: model listing returned 403; testing direct model invocation")
    else:
        aliases = [str(m.get("id", "")) for m in models.get("data", []) if isinstance(m, dict)]
        if MODEL not in aliases:
            raise AssertionError(f"catalog alias {MODEL!r} absent from authorized model list")
        print("PASS: authenticated /v1/models includes configured catalog alias")

    variant_a, variant_b = "variant-probe-blue", "variant-probe-black"
    offer_a, offer_b, offer_base = "offer-probe-blue", "offer-probe-black", "offer-probe-base"
    catalog = {
        "catalogs": [{"id": "catalog-probe", "name": "الالكترونيات", "status": "active"}],
        "attribute_schemas": [{
            "id": "schema-probe", "name": "هاتف", "version": 2,
            "definitions": [
                {"attribute_key": "storage", "label": "سعة التخزين", "data_type": "text"},
                {"attribute_key": "color", "label": "اللون", "data_type": "text"}
            ]
        }],
        "items": [{
            "id": "item-probe", "catalog_id": "catalog-probe",
            "attribute_schema_id": "schema-probe", "attribute_schema_version": 2,
            "name": "هاتف اختبار", "attributes": {"storage": "256GB"},
            "variants": [
                {"id": variant_a, "catalog_item_id": "item-probe",
                 "attributes": {"color": "blue", "storage": "256GB"},
                 "offers": [{"id": offer_a, "catalog_item_id": "item-probe",
                             "variant_id": variant_a, "amount": "12000.0000", "currency": "YER"}]},
                {"id": variant_b, "catalog_item_id": "item-probe",
                 "attributes": {"color": "black", "storage": "256GB"},
                 "offers": [{"id": offer_b, "catalog_item_id": "item-probe",
                             "variant_id": variant_b, "amount": "15000.0000", "currency": "YER"}]}
            ],
            "item_level_offers": [{"id": offer_base, "catalog_item_id": "item-probe",
                                   "amount": "10000.0000", "currency": "YER"}]
        }]
    }
    result_schema = {
        "type": "object",
        "properties": {
            "item_id": {"type": "string"},
            "variant_ids": {"type": "array", "items": {"type": "string"}},
            "offer_ids": {"type": "array", "items": {"type": "string"}},
            "schema_version": {"type": "integer"}
        },
        "required": ["item_id", "variant_ids", "offer_ids", "schema_version"],
        "additionalProperties": False
    }
    payload = {
        "model": MODEL,
        "temperature": 0,
        "max_tokens": 768,
        "messages": [
            {"role": "system", "content":
             "Inspect the entire structured catalog entity. Return exactly every present "
             "item ID, variant ID and offer ID, including item-level offers. "
             "Return no guessed ID. Output only JSON matching the provided strict JSON schema."},
            {"role": "user", "content": json.dumps(catalog, ensure_ascii=False)}
        ],
        "response_format": {"type": "json_schema", "json_schema": {
            "name": "complete_catalog_entity_probe", "strict": True, "schema": result_schema
        }}
    }
    reply = request_json("/v1/chat/completions", payload)
    choices = reply.get("choices") or []
    if not choices:
        raise AssertionError("provider returned no choices")
    message = choices[0].get("message") or {}
    content = message.get("content")
    if not isinstance(content, str) or not content.strip():
        raise AssertionError("provider returned no text JSON content")
    parsed = json.loads(content)
    if parsed.get("item_id") != "item-probe":
        raise AssertionError("item identity was altered or omitted")
    if parsed.get("schema_version") != 2:
        raise AssertionError("schema version was omitted or changed")
    if sorted(parsed.get("variant_ids", [])) != sorted([variant_a, variant_b]):
        raise AssertionError("provider output is missing or inventing a variant")
    if sorted(parsed.get("offer_ids", [])) != sorted([offer_a, offer_b, offer_base]):
        raise AssertionError("provider output is missing or inventing an offer")
    print("PASS: real provider returned all linked variant/offer identities with a strict JSON schema")
    usage = reply.get("usage") or {}
    if usage:
        print("usage:", "prompt_tokens", usage.get("prompt_tokens", "n/a"),
              "completion_tokens", usage.get("completion_tokens", "n/a"))

if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"FAIL: {type(exc).__name__}: {exc}", file=sys.stderr)
        sys.exit(1)
