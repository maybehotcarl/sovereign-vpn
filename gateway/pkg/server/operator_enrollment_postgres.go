package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const operatorEnrollmentColumns = `
	token,
	operator,
	region,
	status,
	created_at,
	expires_at,
	last_report_at,
	report_operator,
	report_region,
	endpoint,
	gateway_url,
	public_ip,
	gateway_port,
	wireguard_port,
	wireguard_public_key,
	health_ok,
	health_status,
	installer_version,
	reported_at
`

type PostgresOperatorEnrollmentStore struct {
	pool *pgxpool.Pool
	ttl  time.Duration
	now  func() time.Time
}

func NewPostgresOperatorEnrollmentStore(ctx context.Context, databaseURL string, ttl time.Duration) (*PostgresOperatorEnrollmentStore, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing enrollment database URL: %w", err)
	}

	// Supabase transaction pooler does not support prepared statements. Simple
	// protocol also works for direct/session connections, so keep it universal.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if cfg.MaxConns == 0 {
		cfg.MaxConns = 5
	}
	if cfg.MinConns == 0 {
		cfg.MinConns = 1
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating enrollment database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to enrollment database: %w", err)
	}

	return &PostgresOperatorEnrollmentStore{
		pool: pool,
		ttl:  ttl,
		now:  time.Now,
	}, nil
}

func (s *PostgresOperatorEnrollmentStore) Create(ctx context.Context, operator string, region string) (*OperatorEnrollment, error) {
	operator, err := normalizeEnrollmentOperator(operator)
	if err != nil {
		return nil, err
	}
	region, err = normalizeEnrollmentRegion(region)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	if err := s.cleanupExpired(ctx, now); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 3; attempt++ {
		token, err := randomEnrollmentToken()
		if err != nil {
			return nil, err
		}
		expiresAt := now.Add(s.ttl)

		row := s.pool.QueryRow(ctx, `
			insert into public.operator_enrollments (
				token,
				operator,
				region,
				status,
				created_at,
				expires_at
			)
			values ($1, $2, $3, 'created', $4, $5)
			returning `+operatorEnrollmentColumns,
			token,
			operator,
			region,
			now,
			expiresAt,
		)
		enrollment, err := scanOperatorEnrollment(row)
		if err == nil {
			return enrollment, nil
		}
		if isUniqueViolation(err) {
			continue
		}
		return nil, fmt.Errorf("creating enrollment: %w", err)
	}

	return nil, fmt.Errorf("creating enrollment: token collision")
}

func (s *PostgresOperatorEnrollmentStore) Get(ctx context.Context, token string) (*OperatorEnrollment, bool, error) {
	token = normalizeEnrollmentToken(token)
	if token == "" {
		return nil, false, nil
	}

	now := s.now().UTC()
	if err := s.cleanupExpired(ctx, now); err != nil {
		return nil, false, err
	}

	row := s.pool.QueryRow(ctx, `
		select `+operatorEnrollmentColumns+`
		from public.operator_enrollments
		where token = $1
			and expires_at > $2
	`, token, now)
	enrollment, err := scanOperatorEnrollment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("fetching enrollment: %w", err)
	}

	return enrollment, true, nil
}

func (s *PostgresOperatorEnrollmentStore) Report(ctx context.Context, token string, report OperatorEnrollmentReport) (*OperatorEnrollment, error) {
	token = normalizeEnrollmentToken(token)
	now := s.now().UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning enrollment report transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := cleanupExpired(ctx, tx, now); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx, `
		select `+operatorEnrollmentColumns+`
		from public.operator_enrollments
		where token = $1
			and expires_at > $2
		for update
	`, token, now)
	record, err := scanOperatorEnrollment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errEnrollmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fetching enrollment for report: %w", err)
	}

	report, err = normalizeEnrollmentReport(record, report, now)
	if err != nil {
		return nil, err
	}

	status := "reported"
	if report.HealthOK {
		status = "healthy"
	}

	row = tx.QueryRow(ctx, `
		update public.operator_enrollments
		set
			status = $2,
			last_report_at = $3,
			report_operator = nullif($4, ''),
			report_region = nullif($5, ''),
			endpoint = nullif($6, ''),
			gateway_url = nullif($7, ''),
			public_ip = nullif($8, ''),
			gateway_port = nullif($9, ''),
			wireguard_port = nullif($10, ''),
			wireguard_public_key = nullif($11, ''),
			health_ok = $12,
			health_status = nullif($13, ''),
			installer_version = nullif($14, ''),
			reported_at = $15
		where token = $1
		returning `+operatorEnrollmentColumns,
		token,
		status,
		now,
		report.Operator,
		report.Region,
		report.Endpoint,
		report.GatewayURL,
		report.PublicIP,
		report.GatewayPort,
		report.WireGuardPort,
		report.WireGuardPubKey,
		report.HealthOK,
		report.HealthStatus,
		report.InstallerVersion,
		report.ReportedAt,
	)
	updated, err := scanOperatorEnrollment(row)
	if err != nil {
		return nil, fmt.Errorf("updating enrollment report: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing enrollment report: %w", err)
	}

	return updated, nil
}

func (s *PostgresOperatorEnrollmentStore) Close() error {
	s.pool.Close()
	return nil
}

func (s *PostgresOperatorEnrollmentStore) cleanupExpired(ctx context.Context, now time.Time) error {
	return cleanupExpired(ctx, s.pool, now)
}

type enrollmentExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func cleanupExpired(ctx context.Context, exec enrollmentExecutor, now time.Time) error {
	if _, err := exec.Exec(ctx, `delete from public.operator_enrollments where expires_at <= $1`, now); err != nil {
		return fmt.Errorf("cleaning expired enrollments: %w", err)
	}
	return nil
}

func normalizeEnrollmentToken(token string) string {
	return truncateEnrollmentField(token, 128)
}

func scanOperatorEnrollment(row pgx.Row) (*OperatorEnrollment, error) {
	var enrollment OperatorEnrollment
	var lastReportAt pgtype.Timestamptz
	var reportOperator pgtype.Text
	var reportRegion pgtype.Text
	var endpoint pgtype.Text
	var gatewayURL pgtype.Text
	var publicIP pgtype.Text
	var gatewayPort pgtype.Text
	var wireGuardPort pgtype.Text
	var wireGuardPubKey pgtype.Text
	var healthOK pgtype.Bool
	var healthStatus pgtype.Text
	var installerVersion pgtype.Text
	var reportedAt pgtype.Timestamptz

	if err := row.Scan(
		&enrollment.Token,
		&enrollment.Operator,
		&enrollment.Region,
		&enrollment.Status,
		&enrollment.CreatedAt,
		&enrollment.ExpiresAt,
		&lastReportAt,
		&reportOperator,
		&reportRegion,
		&endpoint,
		&gatewayURL,
		&publicIP,
		&gatewayPort,
		&wireGuardPort,
		&wireGuardPubKey,
		&healthOK,
		&healthStatus,
		&installerVersion,
		&reportedAt,
	); err != nil {
		return nil, err
	}

	enrollment.CreatedAt = enrollment.CreatedAt.UTC()
	enrollment.ExpiresAt = enrollment.ExpiresAt.UTC()
	if lastReportAt.Valid {
		t := lastReportAt.Time.UTC()
		enrollment.LastReportAt = &t
	}

	if reportedAt.Valid {
		report := &OperatorEnrollmentReport{
			Operator:         textValue(reportOperator),
			Region:           textValue(reportRegion),
			Endpoint:         textValue(endpoint),
			GatewayURL:       textValue(gatewayURL),
			PublicIP:         textValue(publicIP),
			GatewayPort:      textValue(gatewayPort),
			WireGuardPort:    textValue(wireGuardPort),
			WireGuardPubKey:  textValue(wireGuardPubKey),
			HealthOK:         boolValue(healthOK),
			HealthStatus:     textValue(healthStatus),
			InstallerVersion: textValue(installerVersion),
			ReportedAt:       reportedAt.Time.UTC(),
		}
		enrollment.Report = report
	}

	return &enrollment, nil
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func boolValue(value pgtype.Bool) bool {
	return value.Valid && value.Bool
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
