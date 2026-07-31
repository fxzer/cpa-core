package requestevents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type InsertResult struct {
	Inserted int `json:"inserted"`
	Skipped  int `json:"skipped"`
}

type ModelPrice struct {
	Prompt        float64 `json:"prompt"`
	Completion    float64 `json:"completion"`
	Cache         float64 `json:"cache"`
	Source        string  `json:"source,omitempty"`
	SourceModelID string  `json:"sourceModelId,omitempty"`
	RawJSON       string  `json:"rawJson,omitempty"`
	UpdatedAtMS   int64   `json:"updatedAtMs,omitempty"`
	SyncedAtMS    *int64  `json:"syncedAtMs,omitempty"`
}

type ModelPriceSyncResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

type APIKeyAlias struct {
	APIKeyHash  string `json:"apiKeyHash"`
	Alias       string `json:"alias"`
	UpdatedAtMS int64  `json:"updatedAtMs"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init() error {
	statements := []string{
		`pragma journal_mode = WAL`,
		`pragma synchronous = FULL`,
		`pragma busy_timeout = 5000`,
		`pragma foreign_keys = ON`,
		`create table if not exists usage_events (
			id integer primary key autoincrement,
			request_id text,
			event_hash text not null unique,
			timestamp_ms integer not null,
			timestamp text not null,
			provider text,
			model text not null,
			endpoint text,
			method text,
			path text,
			auth_type text,
			auth_index text,
			source text,
			source_hash text,
			api_key_hash text,
			account_snapshot text,
			auth_label_snapshot text,
			auth_file_snapshot text,
			auth_provider_snapshot text,
			auth_snapshot_at_ms integer,
			input_tokens integer not null default 0,
			output_tokens integer not null default 0,
			reasoning_tokens integer not null default 0,
			cached_tokens integer not null default 0,
			cache_tokens integer not null default 0,
			total_tokens integer not null default 0,
			latency_ms integer,
			failed integer not null default 0,
			raw_json text,
			created_at_ms integer not null,
			request_body text,
			response_body text
		)`,
		`create index if not exists idx_usage_events_timestamp on usage_events(timestamp_ms)`,
		`create index if not exists idx_usage_events_request_id on usage_events(request_id)`,
		`create index if not exists idx_usage_events_model on usage_events(model)`,
		`create index if not exists idx_usage_events_auth_index on usage_events(auth_index)`,
		`create index if not exists idx_usage_events_endpoint on usage_events(endpoint)`,
		// 分页查询按 (timestamp_ms desc, id desc) 排序，复合索引避免额外排序与深翻页时的全表扫描。
		`create index if not exists idx_usage_events_ts_id on usage_events(timestamp_ms desc, id desc)`,
		`create table if not exists dead_letter_events (
			id integer primary key autoincrement,
			payload text not null,
			error text not null,
			created_at_ms integer not null
		)`,
		`create table if not exists settings (
			key text primary key,
			value text not null,
			updated_at_ms integer not null
		)`,
		`create table if not exists model_prices (
			model text primary key,
			prompt_per_1m real not null,
			completion_per_1m real not null,
			cache_per_1m real not null,
			source text,
			source_model_id text,
			raw_json text,
			updated_at_ms integer not null,
			synced_at_ms integer
		)`,
		`create table if not exists api_key_aliases (
			api_key_hash text primary key,
			alias text not null,
			updated_at_ms integer not null
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	if err := s.ensureUsageEventSnapshotColumns(); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureUsageEventSnapshotColumns() error {
	rows, err := s.db.Query(`pragma table_info(usage_events)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	existing := map[string]struct{}{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	columns := []struct {
		name       string
		definition string
	}{
		{name: "account_snapshot", definition: "text"},
		{name: "auth_label_snapshot", definition: "text"},
		{name: "auth_file_snapshot", definition: "text"},
		{name: "auth_provider_snapshot", definition: "text"},
		{name: "auth_snapshot_at_ms", definition: "integer"},
		{name: "request_body", definition: "text"},
		{name: "response_body", definition: "text"},
		{name: "fail_body", definition: "text"},
		{name: "fail_status_code", definition: "integer"},
	}
	for _, column := range columns {
		if _, ok := existing[column.name]; ok {
			continue
		}
		if _, err := s.db.Exec(fmt.Sprintf(
			`alter table usage_events add column %s %s`,
			column.name,
			column.definition,
		)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadModelPrices(ctx context.Context) (map[string]ModelPrice, error) {
	rows, err := s.db.QueryContext(ctx, `select
		model, prompt_per_1m, completion_per_1m, cache_per_1m, source, source_model_id, raw_json,
		updated_at_ms, synced_at_ms
		from model_prices order by model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prices := map[string]ModelPrice{}
	for rows.Next() {
		var model string
		var price ModelPrice
		var source, sourceModelID, rawJSON sql.NullString
		var syncedAt sql.NullInt64
		if err := rows.Scan(
			&model,
			&price.Prompt,
			&price.Completion,
			&price.Cache,
			&source,
			&sourceModelID,
			&rawJSON,
			&price.UpdatedAtMS,
			&syncedAt,
		); err != nil {
			return nil, err
		}
		price.Source = source.String
		price.SourceModelID = sourceModelID.String
		price.RawJSON = rawJSON.String
		if syncedAt.Valid {
			value := syncedAt.Int64
			price.SyncedAtMS = &value
		}
		prices[model] = price
	}
	return prices, rows.Err()
}

func (s *Store) SaveModelPrices(ctx context.Context, prices map[string]ModelPrice) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `delete from model_prices`); err != nil {
		return err
	}
	if len(prices) == 0 {
		return tx.Commit()
	}

	stmt, err := tx.PrepareContext(ctx, `insert into model_prices (
		model, prompt_per_1m, completion_per_1m, cache_per_1m, source, source_model_id,
		raw_json, updated_at_ms, synced_at_ms
	) values (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UnixMilli()
	for model, price := range prices {
		if err := validateModelPrice(model, price); err != nil {
			return err
		}
		if _, err := stmt.ExecContext(
			ctx,
			model,
			price.Prompt,
			price.Completion,
			price.Cache,
			nullString(price.Source),
			nullString(price.SourceModelID),
			nullString(price.RawJSON),
			now,
			nullInt(price.SyncedAtMS),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertSyncedModelPrices(ctx context.Context, prices map[string]ModelPrice) (ModelPriceSyncResult, error) {
	if len(prices) == 0 {
		return ModelPriceSyncResult{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModelPriceSyncResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `insert into model_prices (
		model, prompt_per_1m, completion_per_1m, cache_per_1m, source, source_model_id,
		raw_json, updated_at_ms, synced_at_ms
	) values (?, ?, ?, ?, ?, ?, ?, ?, ?)
	on conflict(model) do update set
		prompt_per_1m = excluded.prompt_per_1m,
		completion_per_1m = excluded.completion_per_1m,
		cache_per_1m = excluded.cache_per_1m,
		source = excluded.source,
		source_model_id = excluded.source_model_id,
		raw_json = excluded.raw_json,
		updated_at_ms = excluded.updated_at_ms,
		synced_at_ms = excluded.synced_at_ms`)
	if err != nil {
		return ModelPriceSyncResult{}, err
	}
	defer stmt.Close()

	now := time.Now().UnixMilli()
	result := ModelPriceSyncResult{}
	for model, price := range prices {
		if err := validateModelPrice(model, price); err != nil {
			result.Skipped++
			continue
		}
		if price.Source == "" {
			price.Source = "sync"
		}
		if price.SourceModelID == "" {
			price.SourceModelID = model
		}
		price.UpdatedAtMS = now
		price.SyncedAtMS = &now
		if _, err := stmt.ExecContext(
			ctx,
			model,
			price.Prompt,
			price.Completion,
			price.Cache,
			nullString(price.Source),
			nullString(price.SourceModelID),
			nullString(price.RawJSON),
			now,
			now,
		); err != nil {
			return ModelPriceSyncResult{}, err
		}
		result.Imported++
	}
	if err := tx.Commit(); err != nil {
		return ModelPriceSyncResult{}, err
	}
	return result, nil
}

func (s *Store) LoadAPIKeyAliases(ctx context.Context) ([]APIKeyAlias, error) {
	rows, err := s.db.QueryContext(ctx, `select api_key_hash, alias, updated_at_ms
		from api_key_aliases
		order by alias collate nocase, api_key_hash`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := []APIKeyAlias{}
	for rows.Next() {
		var alias APIKeyAlias
		if err := rows.Scan(&alias.APIKeyHash, &alias.Alias, &alias.UpdatedAtMS); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}

func (s *Store) UpsertAPIKeyAliases(ctx context.Context, aliases []APIKeyAlias) error {
	if len(aliases) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	normalizedAliases := make([]APIKeyAlias, 0, len(aliases))
	seenAliases := map[string]string{}
	for _, alias := range aliases {
		normalized, err := normalizeAPIKeyAlias(alias, now)
		if err != nil {
			return err
		}
		aliasKey := normalizeAPIKeyAliasUniqueKey(normalized.Alias)
		if existingHash, ok := seenAliases[aliasKey]; ok && existingHash != normalized.APIKeyHash {
			return errors.New("api key alias already exists")
		}
		seenAliases[aliasKey] = normalized.APIKeyHash
		normalizedAliases = append(normalizedAliases, normalized)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `insert into api_key_aliases (
		api_key_hash, alias, updated_at_ms
	) values (?, ?, ?)
	on conflict(api_key_hash) do update set
		alias = excluded.alias,
		updated_at_ms = excluded.updated_at_ms`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	existingRows, err := tx.QueryContext(ctx, `select api_key_hash, alias from api_key_aliases`)
	if err != nil {
		return err
	}
	existingAliases := map[string]string{}
	for existingRows.Next() {
		var apiKeyHash string
		var alias string
		if err := existingRows.Scan(&apiKeyHash, &alias); err != nil {
			_ = existingRows.Close()
			return err
		}
		existingAliases[normalizeAPIKeyAliasUniqueKey(alias)] = apiKeyHash
	}
	if err := existingRows.Close(); err != nil {
		return err
	}
	if err := existingRows.Err(); err != nil {
		return err
	}

	for _, normalized := range normalizedAliases {
		aliasKey := normalizeAPIKeyAliasUniqueKey(normalized.Alias)
		if existingHash, ok := existingAliases[aliasKey]; ok && existingHash != normalized.APIKeyHash {
			return errors.New("api key alias already exists")
		}
		if _, err := stmt.ExecContext(
			ctx,
			normalized.APIKeyHash,
			normalized.Alias,
			normalized.UpdatedAtMS,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteAPIKeyAlias(ctx context.Context, apiKeyHash string) error {
	hash := strings.ToLower(strings.TrimSpace(apiKeyHash))
	if !validAPIKeyHash(hash) {
		return errors.New("valid apiKeyHash is required")
	}
	_, err := s.db.ExecContext(ctx, `delete from api_key_aliases where api_key_hash = ?`, hash)
	return err
}

func normalizeAPIKeyAlias(alias APIKeyAlias, now int64) (APIKeyAlias, error) {
	hash := strings.ToLower(strings.TrimSpace(alias.APIKeyHash))
	if !validAPIKeyHash(hash) {
		return APIKeyAlias{}, errors.New("valid apiKeyHash is required")
	}
	label := strings.TrimSpace(alias.Alias)
	if label == "" {
		return APIKeyAlias{}, errors.New("alias is required")
	}
	if len([]rune(label)) > 120 {
		return APIKeyAlias{}, errors.New("alias must be 120 characters or less")
	}
	if alias.UpdatedAtMS <= 0 {
		alias.UpdatedAtMS = now
	}
	alias.APIKeyHash = hash
	alias.Alias = label
	return alias, nil
}

func normalizeAPIKeyAliasUniqueKey(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func validAPIKeyHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return false
	}
	return true
}

func validateModelPrice(model string, price ModelPrice) error {
	if model == "" {
		return errors.New("model is required")
	}
	if !validPriceValue(price.Prompt) || !validPriceValue(price.Completion) || !validPriceValue(price.Cache) {
		return fmt.Errorf("invalid model price for %s", model)
	}
	return nil
}

func validPriceValue(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (s *Store) InsertEvents(ctx context.Context, events []Event) (InsertResult, error) {
	if len(events) == 0 {
		return InsertResult{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InsertResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `insert or ignore into usage_events (
		request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
		auth_type, auth_index, source, source_hash, api_key_hash,
		account_snapshot, auth_label_snapshot, auth_file_snapshot, auth_provider_snapshot, auth_snapshot_at_ms,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
		latency_ms, failed, raw_json, created_at_ms, request_body, response_body,
		fail_body, fail_status_code
	) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return InsertResult{}, err
	}
	defer stmt.Close()

	result := InsertResult{}
	for _, event := range events {
		failed := 0
		if event.Failed {
			failed = 1
		}
		res, err := stmt.ExecContext(
			ctx,
			nullString(event.RequestID),
			event.EventHash,
			event.TimestampMS,
			event.Timestamp,
			nullString(event.Provider),
			event.Model,
			nullString(event.Endpoint),
			nullString(event.Method),
			nullString(event.Path),
			nullString(event.AuthType),
			nullString(event.AuthIndex),
			nullString(event.Source),
			nullString(event.SourceHash),
			nullString(event.APIKeyHash),
			nullString(event.AccountSnapshot),
			nullString(event.AuthLabelSnapshot),
			nullString(event.AuthFileSnapshot),
			nullString(event.AuthProviderSnapshot),
			nullPositiveInt64(event.AuthSnapshotAtMS),
			event.InputTokens,
			event.OutputTokens,
			event.ReasoningTokens,
			event.CachedTokens,
			event.CacheTokens,
			event.TotalTokens,
			nullInt(event.LatencyMS),
			failed,
			nullString(event.RawJSON),
			event.CreatedAtMS,
			nullString(event.RequestBody),
			nullString(event.ResponseBody),
			nullString(event.FailBody),
			failStatusValue(event.FailStatusCode),
		)
		if err != nil {
			return InsertResult{}, err
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			result.Inserted++
		} else {
			result.Skipped++
		}
	}
	if err := tx.Commit(); err != nil {
		return InsertResult{}, err
	}
	return result, nil
}

func (s *Store) AddDeadLetter(ctx context.Context, payload string, parseErr error) error {
	_, err := s.db.ExecContext(
		ctx,
		`insert into dead_letter_events(payload, error, created_at_ms) values(?, ?, ?)`,
		payload,
		parseErr.Error(),
		time.Now().UnixMilli(),
	)
	return err
}

type ListQuery struct {
	StartMS    int64
	EndMS      int64
	Limit      int
	Offset     int
	Model      string
	Provider   string
	SourceHash string
	APIKeyHash string
	Failed     *bool
	Search     string
}

// applyEventFilters 把 ListQuery 里的过滤条件拼到 sqlQuery 上，并追加对应参数。
// 分页查询、count、聚合查询共用这套 WHERE，保证 total/聚合值与列表切片一致。
func applyEventFilters(sqlQuery *string, args *[]any, q ListQuery) {
	if q.StartMS > 0 {
		*sqlQuery += ` and timestamp_ms >= ?`
		*args = append(*args, q.StartMS)
	}
	if q.EndMS > 0 {
		*sqlQuery += ` and timestamp_ms <= ?`
		*args = append(*args, q.EndMS)
	}
	if q.Model != "" {
		*sqlQuery += ` and model = ?`
		*args = append(*args, q.Model)
	}
	if q.Provider != "" {
		*sqlQuery += ` and provider = ?`
		*args = append(*args, q.Provider)
	}
	if q.SourceHash != "" {
		*sqlQuery += ` and source_hash = ?`
		*args = append(*args, q.SourceHash)
	}
	if q.APIKeyHash != "" {
		*sqlQuery += ` and api_key_hash = ?`
		*args = append(*args, q.APIKeyHash)
	}
	if q.Failed != nil {
		if *q.Failed {
			*sqlQuery += ` and failed = 1`
		} else {
			*sqlQuery += ` and failed = 0`
		}
	}
	if search := strings.TrimSpace(q.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		*sqlQuery += ` and (
			lower(ifnull(request_id,'')) like ? or
			lower(ifnull(provider,'')) like ? or
			lower(ifnull(model,'')) like ? or
			lower(ifnull(endpoint,'')) like ? or
			lower(ifnull(path,'')) like ? or
			lower(ifnull(method,'')) like ? or
			lower(ifnull(source,'')) like ? or
			lower(ifnull(api_key_hash,'')) like ? or
			lower(ifnull(account_snapshot,'')) like ? or
			lower(ifnull(auth_label_snapshot,'')) like ? or
			lower(ifnull(auth_file_snapshot,'')) like ? or
			lower(ifnull(auth_index,'')) like ? or
			lower(ifnull(auth_type,'')) like ?)`
		for i := 0; i < 13; i++ {
			*args = append(*args, like)
		}
	}
}

// eventListColumns 是 usage_events 列表查询用到的列。
// 与全量 ListEvents 相比，这里去掉了 request_body/response_body/fail_body 三个大字段
// （表格、统计、热力图都不读它们；body 详情由 GetEventByHash 按需取）。
// 占位的三个空串保持与 scanEventRows 的列顺序一致，scan 函数无需改动。
const eventListColumns = `request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
	auth_type, auth_index, source, source_hash, api_key_hash,
	account_snapshot, auth_label_snapshot, auth_file_snapshot, auth_provider_snapshot, auth_snapshot_at_ms,
	input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
	latency_ms, failed, raw_json, created_at_ms, '' as request_body, '' as response_body, '' as fail_body,
	fail_status_code`

func (s *Store) ListEvents(ctx context.Context, query ListQuery) ([]Event, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 50000
	}

	sqlQuery := `select
		request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
		auth_type, auth_index, source, source_hash, api_key_hash,
		account_snapshot, auth_label_snapshot, auth_file_snapshot, auth_provider_snapshot, auth_snapshot_at_ms,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
		latency_ms, failed, raw_json, created_at_ms, request_body, response_body,
		fail_body, fail_status_code
		from usage_events where 1=1`
	args := make([]any, 0, 4)
	if query.StartMS > 0 {
		sqlQuery += ` and timestamp_ms >= ?`
		args = append(args, query.StartMS)
	}
	if query.EndMS > 0 {
		sqlQuery += ` and timestamp_ms <= ?`
		args = append(args, query.EndMS)
	}
	sqlQuery += ` order by timestamp_ms desc, id desc limit ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEventRows(rows)
}

// PagedEvents 是分页查询结果。Total 为满足过滤条件的总行数（与分页参数无关），
// 用于前端翻页器计算总页数。
type PagedEvents struct {
	Items []Event
	Total int64
}

// ListEventsPaged 按过滤条件分页查询，只取表格/统计需要的列（不含 body 大字段）。
// 排序固定为 timestamp_ms desc, id desc，与原 ListEvents 一致。
func (s *Store) ListEventsPaged(ctx context.Context, query ListQuery) (PagedEvents, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}

	where := ` from usage_events where 1=1`
	args := make([]any, 0, 8)
	applyEventFilters(&where, &args, query)

	// count 与列表共用同一套 WHERE，确保 total 反映过滤后的全集。
	countQuery := `select count(*)` + where
	var total int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return PagedEvents{}, err
	}

	listQuery := `select ` + eventListColumns + where + ` order by timestamp_ms desc, id desc limit ? offset ?`
	listArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return PagedEvents{}, err
	}
	defer rows.Close()
	events, err := scanEventRows(rows)
	if err != nil {
		return PagedEvents{}, err
	}
	return PagedEvents{Items: events, Total: total}, nil
}

// DistinctValues 返回某列的去重非空值，用于前端过滤下拉选项。
// 即使当页数据不包含某个 model/provider，下拉里也能选到。
func (s *Store) DistinctValues(ctx context.Context, column string) ([]string, error) {
	// 仅允许在固定白名单列上取 distinct，避免 SQL 注入。
	allowed := map[string]string{
		"model":        "model",
		"provider":     "provider",
		"source_hash":  "source_hash",
		"api_key_hash": "api_key_hash",
	}
	col, ok := allowed[column]
	if !ok {
		return nil, fmt.Errorf("unsupported distinct column: %s", column)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`select distinct %s from usage_events where %s is not null and %s <> '' order by %s`, col, col, col, col))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

// TimeBucket 是按天/小时聚合的统计单元，供前端趋势图与热力图使用。
type TimeBucket struct {
	BucketMS int64
	Total    int64
	Success  int64
	Failure  int64
	Tokens   int64
}

// AggregateResult 是聚合查询结果：全局计数 + 时间维度分布。
// 仅按时间范围过滤，语义与改造前的全量内存聚合一致（覆盖整个时间窗）。
type AggregateResult struct {
	TotalRequests int64
	SuccessCount  int64
	FailureCount  int64
	TotalTokens   int64
	ByDay         []TimeBucket
	ByHour        []TimeBucket
}

// Aggregate 在指定时间窗内做一次聚合查询，避免把全量行拉到内存里统计。
func (s *Store) Aggregate(ctx context.Context, query ListQuery) (AggregateResult, error) {
	where := ` from usage_events where 1=1`
	args := make([]any, 0, 4)
	// 聚合只认时间范围，忽略其余过滤，统计的是「整个时间窗」而非「当前表格过滤后」。
	// 这样页头总数与热力图反映的是持久化事件的全局状态。
	if query.StartMS > 0 {
		where += ` and timestamp_ms >= ?`
		args = append(args, query.StartMS)
	}
	if query.EndMS > 0 {
		where += ` and timestamp_ms <= ?`
		args = append(args, query.EndMS)
	}

	var result AggregateResult
	summaryQuery := `select count(*),
		sum(case when failed = 0 then 1 else 0 end),
		sum(case when failed = 1 then 1 else 0 end),
		sum(total_tokens)` + where
	if err := s.db.QueryRowContext(ctx, summaryQuery, args...).Scan(
		&result.TotalRequests,
		&result.SuccessCount,
		&result.FailureCount,
		&result.TotalTokens,
	); err != nil {
		return AggregateResult{}, err
	}

	// SQLite 整数除法对 timestamp_ms 取整得到天/小时桶。
	dayBuckets, err := s.queryTimeBuckets(ctx, where, args, 86_400_000)
	if err != nil {
		return AggregateResult{}, err
	}
	result.ByDay = dayBuckets
	hourBuckets, err := s.queryTimeBuckets(ctx, where, args, 3_600_000)
	if err != nil {
		return AggregateResult{}, err
	}
	result.ByHour = hourBuckets
	return result, nil
}

func (s *Store) queryTimeBuckets(ctx context.Context, where string, args []any, spanMs int64) ([]TimeBucket, error) {
	query := fmt.Sprintf(`select (timestamp_ms / %d) * %d as bucket,
		count(*),
		sum(case when failed = 0 then 1 else 0 end),
		sum(case when failed = 1 then 1 else 0 end),
		sum(total_tokens)%s group by bucket order by bucket`, spanMs, spanMs, where)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	buckets := make([]TimeBucket, 0)
	for rows.Next() {
		var b TimeBucket
		if err := rows.Scan(&b.BucketMS, &b.Total, &b.Success, &b.Failure, &b.Tokens); err != nil {
			return nil, err
		}
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

func (s *Store) GetEventByHash(ctx context.Context, hash string) (Event, error) {
	sqlQuery := `select
		request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
		auth_type, auth_index, source, source_hash, api_key_hash,
		account_snapshot, auth_label_snapshot, auth_file_snapshot, auth_provider_snapshot, auth_snapshot_at_ms,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
		latency_ms, failed, raw_json, created_at_ms, request_body, response_body,
		fail_body, fail_status_code
		from usage_events where event_hash = ? limit 1`
	rows, err := s.db.QueryContext(ctx, sqlQuery, hash)
	if err != nil {
		return Event{}, err
	}
	defer rows.Close()
	events, err := scanEventRows(rows)
	if err != nil {
		return Event{}, err
	}
	if len(events) == 0 {
		return Event{}, fmt.Errorf("event not found: %s", hash)
	}
	return events[0], nil
}

func (s *Store) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	return s.ListEvents(ctx, ListQuery{Limit: limit})
}

func scanEventRows(rows *sql.Rows) ([]Event, error) {
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var requestID, provider, endpoint, method, path, authType, authIndex, source, sourceHash, apiKeyHash, accountSnapshot, authLabelSnapshot, authFileSnapshot, authProviderSnapshot, rawJSON sql.NullString
		var authSnapshotAt sql.NullInt64
		var latency sql.NullInt64
		var failed int
		var requestBody, responseBody, failBody sql.NullString
		var failStatusCode sql.NullInt64
		if err := rows.Scan(
			&requestID,
			&event.EventHash,
			&event.TimestampMS,
			&event.Timestamp,
			&provider,
			&event.Model,
			&endpoint,
			&method,
			&path,
			&authType,
			&authIndex,
			&source,
			&sourceHash,
			&apiKeyHash,
			&accountSnapshot,
			&authLabelSnapshot,
			&authFileSnapshot,
			&authProviderSnapshot,
			&authSnapshotAt,
			&event.InputTokens,
			&event.OutputTokens,
			&event.ReasoningTokens,
			&event.CachedTokens,
			&event.CacheTokens,
			&event.TotalTokens,
			&latency,
			&failed,
			&rawJSON,
			&event.CreatedAtMS,
			&requestBody,
			&responseBody,
			&failBody,
			&failStatusCode,
		); err != nil {
			return nil, err
		}
		event.RequestID = requestID.String
		event.Provider = provider.String
		event.Endpoint = endpoint.String
		event.Method = method.String
		event.Path = path.String
		event.AuthType = authType.String
		event.AuthIndex = authIndex.String
		event.Source = source.String
		event.SourceHash = sourceHash.String
		event.APIKeyHash = apiKeyHash.String
		event.AccountSnapshot = accountSnapshot.String
		event.AuthLabelSnapshot = authLabelSnapshot.String
		event.AuthFileSnapshot = authFileSnapshot.String
		event.AuthProviderSnapshot = authProviderSnapshot.String
		if authSnapshotAt.Valid {
			event.AuthSnapshotAtMS = authSnapshotAt.Int64
		}
		event.RawJSON = rawJSON.String
		event.RequestBody = requestBody.String
		event.ResponseBody = responseBody.String
		event.Failed = failed != 0
		event.FailBody = failBody.String
		if failStatusCode.Valid {
			event.FailStatusCode = int(failStatusCode.Int64)
		}
		if event.Alias == "" {
			event.Alias = AliasFromRawJSON(event.RawJSON)
		}
		if latency.Valid {
			value := latency.Int64
			event.LatencyMS = &value
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) DeleteEvents(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = strings.TrimSpace(id)
	}
	query := `delete from usage_events where event_hash in (` + strings.Join(placeholders, ",") + `)`
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MaxBodyStorageBytes is the maximum total size of request_body + response_body
// columns across all rows. When exceeded, the oldest events are trimmed.
const MaxBodyStorageBytes = 50 * 1024 * 1024 // 50 MB

func (s *Store) TrimOldEventsByBodySize(ctx context.Context) error {
	total, err := s.bodyStorageSize(ctx)
	if err != nil {
		return err
	}
	if total <= MaxBodyStorageBytes {
		return nil
	}
	overflow := total - MaxBodyStorageBytes
	// Delete oldest events until we've freed at least the overflow + 10% margin
	target := overflow + MaxBodyStorageBytes/10
	for {
		var freed int64
		err := s.db.QueryRowContext(ctx, `select ifnull(sum(length(coalesce(request_body,'')) + length(coalesce(response_body,''))), 0)
			from (select request_body, response_body from usage_events order by timestamp_ms asc, id asc limit 50)`).Scan(&freed)
		if err != nil || freed == 0 {
			break
		}
		_, err = s.db.ExecContext(ctx, `delete from usage_events where rowid in (
			select rowid from usage_events order by timestamp_ms asc, id asc limit 50)`)
		if err != nil {
			return err
		}
		target -= freed
		if target <= 0 {
			break
		}
	}
	return nil
}

func (s *Store) bodyStorageSize(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx, `select ifnull(sum(length(coalesce(request_body,'')) + length(coalesce(response_body,''))), 0) from usage_events`).Scan(&total)
	return total, err
}

func (s *Store) Counts(ctx context.Context) (events int64, deadLetters int64, err error) {
	if err = s.db.QueryRowContext(ctx, `select count(*) from usage_events`).Scan(&events); err != nil {
		return 0, 0, err
	}
	if err = s.db.QueryRowContext(ctx, `select count(*) from dead_letter_events`).Scan(&deadLetters); err != nil {
		return 0, 0, err
	}
	return events, deadLetters, nil
}

func (s *Store) ExportJSONL(ctx context.Context) ([]byte, error) {
	events, err := s.RecentEvents(ctx, 0)
	if err != nil {
		return nil, err
	}
	output := make([]byte, 0)
	for i := len(events) - 1; i >= 0; i-- {
		line, err := json.Marshal(events[i])
		if err != nil {
			return nil, err
		}
		output = append(output, line...)
		output = append(output, '\n')
	}
	return output, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullPositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func failStatusValue(code int) any {
	if code <= 0 {
		return nil
	}
	return code
}
