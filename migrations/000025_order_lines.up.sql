CREATE TABLE order_lines (
    id                         UUID PRIMARY KEY,
    business_id                UUID NOT NULL,
    transaction_id             UUID NOT NULL,
    catalog_item_id            UUID NOT NULL,
    offer_id                   UUID,
    variant_id                 UUID,
    item_name_snapshot         TEXT NOT NULL,
    selected_attributes_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    quantity                   NUMERIC(20, 4) NOT NULL,
    unit_price_snapshot        NUMERIC(20, 4),
    line_total_snapshot        NUMERIC(20, 4),
    currency                   CHAR(3),
    created_at                 TIMESTAMPTZ NOT NULL,
    updated_at                 TIMESTAMPTZ NOT NULL,

    CONSTRAINT order_lines_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT order_lines_transaction_fk
        FOREIGN KEY (business_id, transaction_id)
        REFERENCES commercial_transactions (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT order_lines_catalog_item_fk
        FOREIGN KEY (business_id, catalog_item_id)
        REFERENCES catalog_items (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT order_lines_offer_same_item_fk
        FOREIGN KEY (business_id, catalog_item_id, offer_id)
        REFERENCES offers (business_id, catalog_item_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT order_lines_variant_same_item_fk
        FOREIGN KEY (business_id, catalog_item_id, variant_id)
        REFERENCES variants (business_id, catalog_item_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT order_lines_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT order_lines_item_name_chk
        CHECK (length(btrim(item_name_snapshot)) > 0),
    CONSTRAINT order_lines_attributes_object_chk
        CHECK (jsonb_typeof(selected_attributes_snapshot) = 'object'),
    CONSTRAINT order_lines_quantity_chk
        CHECK (quantity > 0),
    CONSTRAINT order_lines_unit_price_chk
        CHECK (unit_price_snapshot IS NULL OR unit_price_snapshot >= 0),
    CONSTRAINT order_lines_total_price_chk
        CHECK (line_total_snapshot IS NULL OR line_total_snapshot >= 0),
    CONSTRAINT order_lines_currency_chk
        CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$'),
    CONSTRAINT order_lines_price_currency_chk
        CHECK ((unit_price_snapshot IS NULL AND line_total_snapshot IS NULL) OR currency IS NOT NULL)
);

CREATE INDEX idx_order_lines_business_transaction
    ON order_lines (business_id, transaction_id, id);
