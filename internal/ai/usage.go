package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	db         *sql.DB
	client     *XAIClient
	encryptKey []byte
}

const (
	defaultGlobalLimitUsdTicks int64 = 1000000000000 // $100
	defaultUserLimitUsdTicks   int64 = 250000000000  // $25
	systemAccountID                  = "dean.taskford"
)

func NewService(db *sql.DB, encryptKey []byte) *Service {
	return &Service{
		db:         db,
		client:     NewXAIClient(),
		encryptKey: encryptKey,
	}
}

type Settings struct {
	BaseURL                         string      `json:"base_url"`
	HasToken                        bool        `json:"has_token"`
	MonthlyGlobalLimitUsdTicks      int64       `json:"monthly_global_limit_usd_ticks"`
	MonthlyDefaultUserLimitUsdTicks int64       `json:"monthly_default_user_limit_usd_ticks"`
	UserLimitOverrides              []UserLimit `json:"user_limit_overrides"`
}

type UserLimit struct {
	UserID               string `json:"user_id"`
	Username             string `json:"username"`
	FullName             string `json:"full_name"`
	MonthlyLimitUsdTicks int64  `json:"monthly_limit_usd_ticks"`
}

type UsageSummary struct {
	MonthStartUTC        string             `json:"month_start_utc"`
	MonthEndUTCExclusive string             `json:"month_end_utc_exclusive"`
	GlobalCostUsdTicks   int64              `json:"global_cost_usd_ticks"`
	GlobalTotalTokens    int64              `json:"global_total_tokens"`
	Users                []UsageSummaryUser `json:"users"`
}

type UsageSummaryUser struct {
	UserID                 string `json:"user_id"`
	Username               string `json:"username"`
	FullName               string `json:"full_name"`
	CostUsdTicks           int64  `json:"cost_usd_ticks"`
	TotalTokens            int64  `json:"total_tokens"`
	EffectiveLimitUsdTicks int64  `json:"effective_limit_usd_ticks"`
}

type BudgetSnapshot struct {
	UserSpentUsdTicks   int64 `json:"user_spent_usd_ticks"`
	UserLimitUsdTicks   int64 `json:"user_limit_usd_ticks"`
	GlobalSpentUsdTicks int64 `json:"global_spent_usd_ticks"`
	GlobalLimitUsdTicks int64 `json:"global_limit_usd_ticks"`
}

type UpdateSettingsInput struct {
	BaseURL                         *string
	ApiToken                        *string
	MonthlyGlobalLimitUsdTicks      *int64
	MonthlyDefaultUserLimitUsdTicks *int64
	UserLimitOverrides              []UserLimitOverrideInput
	UpdatedBy                       string
}

type UserLimitOverrideInput struct {
	UserID               string `json:"user_id"`
	MonthlyLimitUsdTicks *int64 `json:"monthly_limit_usd_ticks"`
}

type RecordUsageInput struct {
	UserID         string
	Provider       string
	Model          string
	RequestScope   string
	TotalTokens    int64
	CostInUsdTicks int64
	RequestID      string
}

type BudgetExceededError struct {
	Message string
}

func (e *BudgetExceededError) Error() string { return e.Message }

func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	values, err := s.readConfigValues(ctx, []string{
		"xai_base_url",
		"xai_api_token_ciphertext",
		"xai_monthly_global_limit_usd_ticks",
		"xai_monthly_default_user_limit_usd_ticks",
	})
	if err != nil {
		return Settings{}, err
	}

	userLimits, err := s.listUserLimitOverrides(ctx)
	if err != nil {
		return Settings{}, err
	}

	return Settings{
		BaseURL:                         fallback(values["xai_base_url"], "https://api.x.ai/v1"),
		HasToken:                        strings.TrimSpace(values["xai_api_token_ciphertext"]) != "",
		MonthlyGlobalLimitUsdTicks:      parseInt64Or(values["xai_monthly_global_limit_usd_ticks"], defaultGlobalLimitUsdTicks),
		MonthlyDefaultUserLimitUsdTicks: parseInt64Or(values["xai_monthly_default_user_limit_usd_ticks"], defaultUserLimitUsdTicks),
		UserLimitOverrides:              userLimits,
	}, nil
}

func (s *Service) UpdateSettings(ctx context.Context, input UpdateSettingsInput) error {
	now := time.Now().UTC()
	updatedBy := strings.TrimSpace(input.UpdatedBy)
	if updatedBy == "" {
		updatedBy = systemAccountID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if input.BaseURL != nil {
		trimmed := strings.TrimSpace(*input.BaseURL)
		if trimmed == "" {
			return errors.New("base_url cannot be empty")
		}
		if _, err := upsertConfigTx(tx, "xai_base_url", trimmed, updatedBy, now); err != nil {
			return err
		}
	}

	if input.ApiToken != nil {
		trimmed := strings.TrimSpace(*input.ApiToken)
		if trimmed == "" {
			return errors.New("api_token cannot be empty")
		}
		if len(s.encryptKey) == 0 {
			return errors.New("XAI_TOKEN_ENCRYPTION_KEY is required to store token")
		}
		enc, err := EncryptToken(trimmed, s.encryptKey)
		if err != nil {
			return err
		}
		if _, err := upsertConfigTx(tx, "xai_api_token_ciphertext", enc, updatedBy, now); err != nil {
			return err
		}
	}

	if input.MonthlyGlobalLimitUsdTicks != nil {
		if *input.MonthlyGlobalLimitUsdTicks < 0 {
			return errors.New("monthly_global_limit_usd_ticks must be >= 0")
		}
		if _, err := upsertConfigTx(tx, "xai_monthly_global_limit_usd_ticks", strconv.FormatInt(*input.MonthlyGlobalLimitUsdTicks, 10), updatedBy, now); err != nil {
			return err
		}
	}

	if input.MonthlyDefaultUserLimitUsdTicks != nil {
		if *input.MonthlyDefaultUserLimitUsdTicks < 0 {
			return errors.New("monthly_default_user_limit_usd_ticks must be >= 0")
		}
		if _, err := upsertConfigTx(tx, "xai_monthly_default_user_limit_usd_ticks", strconv.FormatInt(*input.MonthlyDefaultUserLimitUsdTicks, 10), updatedBy, now); err != nil {
			return err
		}
	}

	for _, override := range input.UserLimitOverrides {
		if strings.TrimSpace(override.UserID) == "" {
			return errors.New("user_id is required for user limit override")
		}
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ?", override.UserID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("user not found: %s", override.UserID)
		}

		if override.MonthlyLimitUsdTicks == nil {
			if _, err := tx.ExecContext(ctx, "DELETE FROM user_ai_limits WHERE user_id = ?", override.UserID); err != nil {
				return err
			}
			continue
		}
		if *override.MonthlyLimitUsdTicks < 0 {
			return errors.New("monthly_limit_usd_ticks must be >= 0")
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO user_ai_limits (user_id, monthly_limit_usd_ticks, created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET monthly_limit_usd_ticks = excluded.monthly_limit_usd_ticks, updated_by = excluded.updated_by, updated_at = excluded.updated_at",
			override.UserID, *override.MonthlyLimitUsdTicks, updatedBy, now, updatedBy, now,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Service) TestConfiguredToken(ctx context.Context) (UsageMetrics, error) {
	baseURL, token, err := s.ResolveStreamingConfig(ctx)
	if err != nil {
		return UsageMetrics{}, err
	}
	return s.client.TestTokenWithUsage(ctx, baseURL, token)
}

func (s *Service) ResolveStreamingConfig(ctx context.Context) (string, string, error) {
	values, err := s.readConfigValues(ctx, []string{"xai_base_url", "xai_api_token_ciphertext"})
	if err != nil {
		return "", "", err
	}

	tokenCiphertext := strings.TrimSpace(values["xai_api_token_ciphertext"])
	if tokenCiphertext == "" {
		return "", "", errors.New("xai token is not configured")
	}
	if len(s.encryptKey) == 0 {
		return "", "", errors.New("XAI_TOKEN_ENCRYPTION_KEY is required to decrypt token")
	}
	token, err := DecryptToken(tokenCiphertext, s.encryptKey)
	if err != nil {
		return "", "", err
	}

	baseURL := fallback(values["xai_base_url"], "https://api.x.ai/v1")
	return baseURL, token, nil
}

func (s *Service) GetUsageSummary(ctx context.Context) (UsageSummary, error) {
	startUTC, endUTC := currentUTCMonthRange()
	var globalCost int64
	var globalTokens int64
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT COALESCE(SUM(cost_in_usd_ticks), 0), COALESCE(SUM(total_tokens), 0) FROM ai_usage_events WHERE created_at >= ? AND created_at < ?",
		startUTC, endUTC,
	).Scan(&globalCost, &globalTokens); err != nil {
		return UsageSummary{}, err
	}

	defaultLimit := s.defaultUserLimit(ctx)
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.full_name,
		       COALESCE(SUM(e.cost_in_usd_ticks), 0) AS cost_usd_ticks,
		       COALESCE(SUM(e.total_tokens), 0) AS total_tokens,
		       COALESCE(l.monthly_limit_usd_ticks, ?)
		FROM users u
		LEFT JOIN ai_usage_events e ON e.user_id = u.id AND e.created_at >= ? AND e.created_at < ?
		LEFT JOIN user_ai_limits l ON l.user_id = u.id
		GROUP BY u.id, u.username, u.full_name, l.monthly_limit_usd_ticks
		ORDER BY cost_usd_ticks DESC, u.username ASC
	`, defaultLimit, startUTC, endUTC)
	if err != nil {
		return UsageSummary{}, err
	}
	defer rows.Close()

	users := make([]UsageSummaryUser, 0)
	for rows.Next() {
		var row UsageSummaryUser
		if err := rows.Scan(&row.UserID, &row.Username, &row.FullName, &row.CostUsdTicks, &row.TotalTokens, &row.EffectiveLimitUsdTicks); err != nil {
			return UsageSummary{}, err
		}
		users = append(users, row)
	}
	if err := rows.Err(); err != nil {
		return UsageSummary{}, err
	}

	return UsageSummary{
		MonthStartUTC:        startUTC.Format(time.RFC3339),
		MonthEndUTCExclusive: endUTC.Format(time.RFC3339),
		GlobalCostUsdTicks:   globalCost,
		GlobalTotalTokens:    globalTokens,
		Users:                users,
	}, nil
}

func (s *Service) CheckBudgetBeforeCall(ctx context.Context, userID string) (BudgetSnapshot, error) {
	startUTC, endUTC := currentUTCMonthRange()

	settings, err := s.GetSettings(ctx)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	userLimit := settings.MonthlyDefaultUserLimitUsdTicks
	_ = s.db.QueryRowContext(ctx, "SELECT monthly_limit_usd_ticks FROM user_ai_limits WHERE user_id = ?", userID).Scan(&userLimit)

	var userSpent int64
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT COALESCE(SUM(cost_in_usd_ticks), 0) FROM ai_usage_events WHERE user_id = ? AND created_at >= ? AND created_at < ?",
		userID, startUTC, endUTC,
	).Scan(&userSpent); err != nil {
		return BudgetSnapshot{}, err
	}

	var globalSpent int64
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT COALESCE(SUM(cost_in_usd_ticks), 0) FROM ai_usage_events WHERE created_at >= ? AND created_at < ?",
		startUTC, endUTC,
	).Scan(&globalSpent); err != nil {
		return BudgetSnapshot{}, err
	}

	snapshot := BudgetSnapshot{
		UserSpentUsdTicks:   userSpent,
		UserLimitUsdTicks:   userLimit,
		GlobalSpentUsdTicks: globalSpent,
		GlobalLimitUsdTicks: settings.MonthlyGlobalLimitUsdTicks,
	}

	if userLimit > 0 && userSpent >= userLimit {
		return snapshot, &BudgetExceededError{Message: "user monthly AI budget exceeded"}
	}
	if settings.MonthlyGlobalLimitUsdTicks > 0 && globalSpent >= settings.MonthlyGlobalLimitUsdTicks {
		return snapshot, &BudgetExceededError{Message: "system monthly AI budget exceeded"}
	}

	return snapshot, nil
}

func (s *Service) RecordUsage(ctx context.Context, input RecordUsageInput) error {
	if strings.TrimSpace(input.UserID) == "" {
		input.UserID = systemAccountID
	}
	if strings.TrimSpace(input.Provider) == "" {
		input.Provider = "xai"
	}
	if strings.TrimSpace(input.Model) == "" {
		input.Model = "unknown"
	}
	if strings.TrimSpace(input.RequestScope) == "" {
		input.RequestScope = "unspecified"
	}
	if input.TotalTokens < 0 || input.CostInUsdTicks < 0 {
		return errors.New("usage values must be >= 0")
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO ai_usage_events
			(id, user_id, provider, model, request_scope, total_tokens, cost_in_usd_ticks, request_id, created_by, created_at, updated_by, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(),
		input.UserID,
		input.Provider,
		input.Model,
		input.RequestScope,
		input.TotalTokens,
		input.CostInUsdTicks,
		input.RequestID,
		input.UserID,
		now,
		input.UserID,
		now,
	)
	return err
}

func (s *Service) listUserLimitOverrides(ctx context.Context) ([]UserLimit, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.user_id, u.username, u.full_name, l.monthly_limit_usd_ticks
		FROM user_ai_limits l
		JOIN users u ON u.id = l.user_id
		ORDER BY u.username ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UserLimit, 0)
	for rows.Next() {
		var row UserLimit
		if err := rows.Scan(&row.UserID, &row.Username, &row.FullName, &row.MonthlyLimitUsdTicks); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Service) defaultUserLimit(ctx context.Context) int64 {
	values, err := s.readConfigValues(ctx, []string{"xai_monthly_default_user_limit_usd_ticks"})
	if err != nil {
		return defaultUserLimitUsdTicks
	}
	return parseInt64Or(values["xai_monthly_default_user_limit_usd_ticks"], defaultUserLimitUsdTicks)
}

func (s *Service) readConfigValues(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}

	placeholders := strings.Repeat("?,", len(keys))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query := "SELECT key, value FROM app_config WHERE key IN (" + placeholders + ")"
	args := make([]interface{}, len(keys))
	for i, key := range keys {
		args[i] = key
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make(map[string]string, len(keys))
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

func upsertConfigTx(tx *sql.Tx, key, value, updatedBy string, now time.Time) (sql.Result, error) {
	return tx.Exec(
		"INSERT INTO app_config (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at",
		key, value, updatedBy, now,
	)
}

func currentUTCMonthRange() (time.Time, time.Time) {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func parseInt64Or(raw string, fallbackValue int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return fallbackValue
	}
	return parsed
}

func fallback(value, fallbackValue string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallbackValue
	}
	return trimmed
}
