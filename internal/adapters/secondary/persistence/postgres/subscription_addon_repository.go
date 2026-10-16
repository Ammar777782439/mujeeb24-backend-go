package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SubscriptionAddonRepository struct{ adapter *Adapter }

func NewSubscriptionAddonRepository(adapter *Adapter) *SubscriptionAddonRepository {
	return &SubscriptionAddonRepository{adapter: adapter}
}

const addonCols = `id::text,subscription_id::text,business_id::text,kind,status,price_yer_snapshot,ai_reply_delta,catalog_delta,seat_delta,channel_delta,activated_at,cancelled_at,cancelled_reason,created_at`

func scanAddon(s interface{ Scan(...any) error }, r *ports.SubscriptionAddonRecord) error {
	return s.Scan(&r.ID, &r.SubscriptionID, &r.BusinessID, &r.Kind, &r.Status, &r.PriceYERSnapshot, &r.AIReplyDelta, &r.CatalogDelta, &r.SeatDelta, &r.ChannelDelta, &r.ActivatedAt, &r.CancelledAt, &r.CancelledReason, &r.CreatedAt)
}

func addonTerms(kind string) (price, replies, catalog, seats, channels int, ok bool) {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "AI_REPLIES_100":
		return 1500, 100, 0, 0, 0, true
	case "AI_REPLIES_500":
		return 6000, 500, 0, 0, 0, true
	case "CATALOG_PRODUCTS_100":
		return 1500, 0, 100, 0, 0, true
	case "EXTRA_SEAT":
		return 500, 0, 0, 1, 0, true
	case "CHANNEL_PASS":
		return 5000, 0, 0, 0, 1, true
	default:
		return 0, 0, 0, 0, 0, false
	}
}

func (r *SubscriptionAddonRepository) CreatePending(ctx context.Context, c ports.SubscriptionAddonCreate) (ports.SubscriptionAddonRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionAddonRecord{}, ErrPoolClosed
	}
	p, replies, catalog, seats, channels, ok := addonTerms(c.Kind)
	if !ok || strings.TrimSpace(c.SubscriptionID) == "" || strings.TrimSpace(c.BusinessID) == "" || strings.TrimSpace(c.CreatedBy) == "" || strings.TrimSpace(c.IdempotencyKey) == "" {
		return ports.SubscriptionAddonRecord{}, invalidRepositoryInput("subscription_addon.create", "invalid addon request")
	}
	if c.Now.IsZero() {
		c.Now = time.Now().UTC()
	}
	kind := strings.ToUpper(strings.TrimSpace(c.Kind))
	var out ports.SubscriptionAddonRecord
	err := r.adapter.Within(ctx, func(tx context.Context) error {
		x, err := r.adapter.Executor(tx)
		if err != nil {
			return err
		}
		var included, ceiling int
		var status string
		var start, end time.Time
		if err = x.QueryRow(tx, `SELECT included_channel_limit,channel_ceiling,status,period_start,period_end FROM subscriptions WHERE id=$1::uuid AND business_id=$2::uuid FOR UPDATE`, c.SubscriptionID, c.BusinessID).Scan(&included, &ceiling, &status, &start, &end); err != nil {
			return err
		}
		if status != "ACTIVE" || c.Now.Before(start) || !c.Now.Before(end) {
			return &RepositoryError{Operation: "subscription_addon.create", Kind: RepositoryConflict, Err: errors.New("active current subscription is required")}
		}

		var previousKind string
		err = x.QueryRow(tx, `SELECT kind FROM subscription_addons WHERE subscription_id=$1::uuid AND created_by=$2::uuid AND creation_idempotency_key=$3`, c.SubscriptionID, c.CreatedBy, c.IdempotencyKey).Scan(&previousKind)
		if err == nil {
			if previousKind != kind {
				return &RepositoryError{Operation: "subscription_addon.create", Kind: RepositoryConflict, Err: errors.New("idempotency key was already used for a different addon")}
			}
			return scanAddon(x.QueryRow(tx, `SELECT `+addonCols+` FROM subscription_addons WHERE subscription_id=$1::uuid AND created_by=$2::uuid AND creation_idempotency_key=$3`, c.SubscriptionID, c.CreatedBy, c.IdempotencyKey), &out)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if kind == "CHANNEL_PASS" {
			var approved int
			err = x.QueryRow(tx, `SELECT max_channel_passes FROM subscription_channel_pass_capacity_approvals WHERE subscription_id=$1::uuid AND business_id=$2::uuid`, c.SubscriptionID, c.BusinessID).Scan(&approved)
			if errors.Is(err, pgx.ErrNoRows) {
				return &RepositoryError{Operation: "subscription_addon.create", Kind: RepositoryConflict, Err: errors.New("channel pass requires a recorded provider-capacity approval")}
			}
			if err != nil {
				return err
			}
			var existing int
			if err = x.QueryRow(tx, `SELECT COALESCE(sum(channel_delta),0)::int FROM subscription_addons WHERE subscription_id=$1::uuid AND status IN ('PENDING','ACTIVE')`, c.SubscriptionID).Scan(&existing); err != nil {
				return err
			}
			if included+existing >= ceiling {
				return &RepositoryError{Operation: "subscription_addon.create", Kind: RepositoryConflict, Err: errors.New("plan channel ceiling reached")}
			}
			if existing >= approved {
				return &RepositoryError{Operation: "subscription_addon.create", Kind: RepositoryConflict, Err: errors.New("recorded provider capacity has been fully allocated")}
			}
		}
		return scanAddon(x.QueryRow(tx, `INSERT INTO subscription_addons (id,subscription_id,business_id,kind,price_yer_snapshot,ai_reply_delta,catalog_delta,seat_delta,channel_delta,status,created_by,creation_idempotency_key,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,'PENDING',$10::uuid,$11,$12,$12) RETURNING `+addonCols, uuid.NewString(), c.SubscriptionID, c.BusinessID, kind, p, replies, catalog, seats, channels, c.CreatedBy, c.IdempotencyKey, c.Now), &out)
	})
	if err != nil {
		return ports.SubscriptionAddonRecord{}, classifyRepositoryWriteError("subscription_addon.create", err)
	}
	return out, nil
}

func (r *SubscriptionAddonRepository) RecordPaymentAndActivate(ctx context.Context, p ports.SubscriptionAddonPayment) (ports.SubscriptionAddonRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionAddonRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(p.AddonID) == "" || strings.TrimSpace(p.RecordedBy) == "" || strings.TrimSpace(p.IdempotencyKey) == "" || strings.TrimSpace(p.Reference) == "" || p.AmountYER <= 0 {
		return ports.SubscriptionAddonRecord{}, invalidRepositoryInput("subscription_addon.payment", "invalid payment request")
	}
	if p.Now.IsZero() {
		p.Now = time.Now().UTC()
	}
	if p.PaidAt.IsZero() {
		p.PaidAt = p.Now
	}
	method := strings.ToUpper(strings.TrimSpace(p.Method))
	if method != "CASH" && method != "BANK_TRANSFER" && method != "MOBILE_MONEY" && method != "OTHER" {
		return ports.SubscriptionAddonRecord{}, invalidRepositoryInput("subscription_addon.payment", "invalid method")
	}
	var out ports.SubscriptionAddonRecord
	err := r.adapter.Within(ctx, func(tx context.Context) error {
		x, err := r.adapter.Executor(tx)
		if err != nil {
			return err
		}

		var recordedAddonID, recordedMethod, recordedReference string
		var recordedAmount int
		err = x.QueryRow(tx, `SELECT addon_id::text,amount_yer,method,reference FROM subscription_addon_payments WHERE recorded_by=$1::uuid AND idempotency_key=$2 FOR UPDATE`, p.RecordedBy, p.IdempotencyKey).Scan(&recordedAddonID, &recordedAmount, &recordedMethod, &recordedReference)
		if err == nil {
			if recordedAddonID != p.AddonID || recordedAmount != p.AmountYER || recordedMethod != method || recordedReference != p.Reference {
				return &RepositoryError{Operation: "subscription_addon.payment", Kind: RepositoryConflict, Err: errors.New("idempotency key was already used for a different payment")}
			}
			return scanAddon(x.QueryRow(tx, `SELECT `+addonCols+` FROM subscription_addons WHERE id=$1::uuid`, p.AddonID), &out)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		var due int
		var addonStatus, subscriptionStatus string
		var start, end time.Time
		err = x.QueryRow(tx, `SELECT a.price_yer_snapshot,a.status,s.status,s.period_start,s.period_end
			FROM subscription_addons a JOIN subscriptions s ON s.id=a.subscription_id
			WHERE a.id=$1::uuid FOR UPDATE OF a,s`, p.AddonID).Scan(&due, &addonStatus, &subscriptionStatus, &start, &end)
		if err != nil {
			return err
		}
		if p.AmountYER != due {
			return &RepositoryError{Operation: "subscription_addon.payment", Kind: RepositoryConflict, Err: errors.New("amount must equal addon price")}
		}
		if subscriptionStatus != "ACTIVE" || p.Now.Before(start) || !p.Now.Before(end) {
			return &RepositoryError{Operation: "subscription_addon.payment", Kind: RepositoryConflict, Err: errors.New("active current subscription is required to activate an addon")}
		}
		if addonStatus != "PENDING" {
			return &RepositoryError{Operation: "subscription_addon.payment", Kind: RepositoryConflict, Err: errors.New("addon is not pending")}
		}
		command, err := x.Exec(tx, `INSERT INTO subscription_addon_payments (id,addon_id,business_id,amount_yer,method,reference,paid_at,recorded_by,idempotency_key,created_at) SELECT $1::uuid,id,business_id,$2,$3,$4,$5,$6::uuid,$7,$8 FROM subscription_addons WHERE id=$9::uuid ON CONFLICT DO NOTHING`, uuid.NewString(), p.AmountYER, method, p.Reference, p.PaidAt, p.RecordedBy, p.IdempotencyKey, p.Now, p.AddonID)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return &RepositoryError{Operation: "subscription_addon.payment", Kind: RepositoryConflict, Err: errors.New("a payment has already been recorded for this addon")}
		}
		return scanAddon(x.QueryRow(tx, `UPDATE subscription_addons SET status='ACTIVE',activated_at=$2,updated_at=$2 WHERE id=$1::uuid AND status='PENDING' RETURNING `+addonCols, p.AddonID, p.Now), &out)
	})
	if err != nil {
		return ports.SubscriptionAddonRecord{}, classifyRepositoryWriteError("subscription_addon.payment", err)
	}
	return out, nil
}

func (r *SubscriptionAddonRepository) Cancel(ctx context.Context, c ports.SubscriptionAddonCancel) (ports.SubscriptionAddonRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionAddonRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(c.AddonID) == "" || strings.TrimSpace(c.CancelledBy) == "" || strings.TrimSpace(c.Reason) == "" {
		return ports.SubscriptionAddonRecord{}, invalidRepositoryInput("subscription_addon.cancel", "addon id, cancellation reason, and actor are required")
	}
	if c.Now.IsZero() {
		c.Now = time.Now().UTC()
	}
	var out ports.SubscriptionAddonRecord
	err := r.adapter.Within(ctx, func(tx context.Context) error {
		x, err := r.adapter.Executor(tx)
		if err != nil {
			return err
		}
		var subscriptionID, businessID, status string
		if err = x.QueryRow(tx, `SELECT subscription_id::text,business_id::text,status FROM subscription_addons WHERE id=$1::uuid FOR UPDATE`, c.AddonID).Scan(&subscriptionID, &businessID, &status); err != nil {
			return err
		}
		if status == "CANCELLED" {
			return scanAddon(x.QueryRow(tx, `SELECT `+addonCols+` FROM subscription_addons WHERE id=$1::uuid`, c.AddonID), &out)
		}
		if status != "PENDING" && status != "ACTIVE" {
			return &RepositoryError{Operation: "subscription_addon.cancel", Kind: RepositoryConflict, Err: errors.New("addon cannot be cancelled from its current state")}
		}
		if status == "ACTIVE" {
			var catalogLimit, channelLimit, seatLimit, activeCatalog, activeChannels, activeSeats int
			if err = x.QueryRow(tx, `SELECT s.ai_catalog_limit + COALESCE(sum(a.catalog_delta) FILTER (WHERE a.status='ACTIVE' AND a.id<>$1::uuid),0), s.included_channel_limit + COALESCE(sum(a.channel_delta) FILTER (WHERE a.status='ACTIVE' AND a.id<>$1::uuid),0), s.seat_limit + COALESCE(sum(a.seat_delta) FILTER (WHERE a.status='ACTIVE' AND a.id<>$1::uuid),0) FROM subscriptions s LEFT JOIN subscription_addons a ON a.subscription_id=s.id WHERE s.id=$2::uuid GROUP BY s.id`, c.AddonID, subscriptionID).Scan(&catalogLimit, &channelLimit, &seatLimit); err != nil {
				return err
			}
			if err = x.QueryRow(tx, `SELECT COUNT(*)::int FROM catalog_items WHERE business_id=$1::uuid AND status='active'`, businessID).Scan(&activeCatalog); err != nil {
				return err
			}
			if err = x.QueryRow(tx, `SELECT COUNT(*)::int FROM channel_connections WHERE business_id=$1::uuid AND status='active'`, businessID).Scan(&activeChannels); err != nil {
				return err
			}
			if err = x.QueryRow(tx, `SELECT COUNT(*)::int FROM business_memberships WHERE business_id=$1::uuid AND status='active'`, businessID).Scan(&activeSeats); err != nil {
				return err
			}
			if activeCatalog > catalogLimit || activeChannels > channelLimit || activeSeats > seatLimit {
				return &RepositoryError{Operation: "subscription_addon.cancel", Kind: RepositoryConflict, Err: fmt.Errorf("deactivate catalog items, channels, or seats before removing this addon")}
			}
		}
		return scanAddon(x.QueryRow(tx, `UPDATE subscription_addons SET status='CANCELLED',cancelled_at=$2,cancelled_by=$3::uuid,cancelled_reason=$4,updated_at=$2 WHERE id=$1::uuid RETURNING `+addonCols, c.AddonID, c.Now, c.CancelledBy, c.Reason), &out)
	})
	if err != nil {
		return ports.SubscriptionAddonRecord{}, classifyRepositoryWriteError("subscription_addon.cancel", err)
	}
	return out, nil
}

func (r *SubscriptionAddonRepository) List(ctx context.Context, filter ports.SubscriptionAddonListFilter) (ports.SubscriptionAddonPage, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionAddonPage{}, ErrPoolClosed
	}
	if strings.TrimSpace(filter.SubscriptionID) == "" {
		return ports.SubscriptionAddonPage{}, invalidRepositoryInput("subscription_addon.list", "subscription id is required")
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	var cursorAt any
	var cursorID any
	if strings.TrimSpace(filter.Cursor) != "" {
		decoded, err := decodeAddonCursor(filter.Cursor)
		if err != nil {
			return ports.SubscriptionAddonPage{}, invalidRepositoryInput("subscription_addon.list", err.Error())
		}
		cursorAt, cursorID = decoded.createdAt, decoded.id
	}
	x, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionAddonPage{}, err
	}
	rows, err := x.Query(ctx, `SELECT `+addonCols+` FROM subscription_addons WHERE subscription_id=$1::uuid AND ($2='' OR status=$2) AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, filter.SubscriptionID, strings.ToUpper(strings.TrimSpace(filter.Status)), cursorAt, cursorID, filter.Limit+1)
	if err != nil {
		return ports.SubscriptionAddonPage{}, &RepositoryError{Operation: "subscription_addon.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	page := ports.SubscriptionAddonPage{Items: make([]ports.SubscriptionAddonRecord, 0, filter.Limit)}
	for rows.Next() {
		var record ports.SubscriptionAddonRecord
		if err := scanAddon(rows, &record); err != nil {
			return ports.SubscriptionAddonPage{}, &RepositoryError{Operation: "subscription_addon.list", Kind: RepositoryInvalid, Err: err}
		}
		page.Items = append(page.Items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.SubscriptionAddonPage{}, &RepositoryError{Operation: "subscription_addon.list", Kind: RepositoryInvalid, Err: err}
	}
	if len(page.Items) > filter.Limit {
		page.HasMore = true
		page.Items = page.Items[:filter.Limit]
		page.NextCursor = encodeAddonCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *SubscriptionAddonRepository) ApproveChannelPassCapacity(ctx context.Context, approval ports.ChannelPassCapacityApproval) (ports.ChannelPassCapacityApprovalRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelPassCapacityApprovalRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(approval.SubscriptionID) == "" || strings.TrimSpace(approval.BusinessID) == "" || strings.TrimSpace(approval.ApprovedBy) == "" || strings.TrimSpace(approval.Reason) == "" || approval.MaxChannelPasses <= 0 {
		return ports.ChannelPassCapacityApprovalRecord{}, invalidRepositoryInput("subscription_addon.approve_channel_capacity", "invalid channel capacity approval")
	}
	if approval.Now.IsZero() {
		approval.Now = time.Now().UTC()
	}
	var out ports.ChannelPassCapacityApprovalRecord
	err := r.adapter.Within(ctx, func(tx context.Context) error {
		x, err := r.adapter.Executor(tx)
		if err != nil {
			return err
		}
		var included, ceiling int
		var status string
		var start, end time.Time
		if err = x.QueryRow(tx, `SELECT included_channel_limit,channel_ceiling,status,period_start,period_end FROM subscriptions WHERE id=$1::uuid AND business_id=$2::uuid FOR UPDATE`, approval.SubscriptionID, approval.BusinessID).Scan(&included, &ceiling, &status, &start, &end); err != nil {
			return err
		}
		if status != "ACTIVE" || approval.Now.Before(start) || !approval.Now.Before(end) {
			return &RepositoryError{Operation: "subscription_addon.approve_channel_capacity", Kind: RepositoryConflict, Err: errors.New("active current subscription is required")}
		}
		if approval.MaxChannelPasses > ceiling-included {
			return &RepositoryError{Operation: "subscription_addon.approve_channel_capacity", Kind: RepositoryConflict, Err: errors.New("approved channel capacity exceeds the plan channel ceiling")}
		}
		var allocated int
		if err = x.QueryRow(tx, `SELECT COALESCE(sum(channel_delta),0)::int FROM subscription_addons WHERE subscription_id=$1::uuid AND status IN ('PENDING','ACTIVE')`, approval.SubscriptionID).Scan(&allocated); err != nil {
			return err
		}
		if approval.MaxChannelPasses < allocated {
			return &RepositoryError{Operation: "subscription_addon.approve_channel_capacity", Kind: RepositoryConflict, Err: errors.New("approved channel capacity cannot be lower than allocated channel passes")}
		}
		return x.QueryRow(tx, `INSERT INTO subscription_channel_pass_capacity_approvals (subscription_id,business_id,max_channel_passes,reason,approved_by,approved_at,updated_at) VALUES ($1::uuid,$2::uuid,$3,$4,$5::uuid,$6,$6) ON CONFLICT (subscription_id) DO UPDATE SET max_channel_passes=EXCLUDED.max_channel_passes,reason=EXCLUDED.reason,approved_by=EXCLUDED.approved_by,approved_at=EXCLUDED.approved_at,updated_at=EXCLUDED.updated_at RETURNING subscription_id::text,business_id::text,max_channel_passes,reason,approved_by::text,approved_at,updated_at`, approval.SubscriptionID, approval.BusinessID, approval.MaxChannelPasses, approval.Reason, approval.ApprovedBy, approval.Now).Scan(&out.SubscriptionID, &out.BusinessID, &out.MaxChannelPasses, &out.Reason, &out.ApprovedBy, &out.ApprovedAt, &out.UpdatedAt)
	})
	if err != nil {
		return ports.ChannelPassCapacityApprovalRecord{}, classifyRepositoryWriteError("subscription_addon.approve_channel_capacity", err)
	}
	return out, nil
}

type addonCursor struct {
	createdAt time.Time
	id        string
}

func encodeAddonCursor(record ports.SubscriptionAddonRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}

func decodeAddonCursor(value string) (*addonCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 || uuid.Validate(parts[1]) != nil {
		return nil, errors.New("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("invalid cursor")
	}
	return &addonCursor{createdAt: at, id: parts[1]}, nil
}

var _ ports.SubscriptionAddonRepository = (*SubscriptionAddonRepository)(nil)
