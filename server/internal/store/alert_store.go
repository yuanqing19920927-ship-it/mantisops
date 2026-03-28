package store

import (
	"database/sql"
	"mantisops/server/internal/model"
	"strings"
	"time"
)

type AlertStore struct {
	db *sql.DB
}

func NewAlertStore(db *sql.DB) *AlertStore {
	return &AlertStore{db: db}
}

// PendingNotification 待发送通知（JOIN 查询结果）
type PendingNotification struct {
	ID         int
	NotifyType string
	Event      model.AlertEvent
	Channel    model.NotificationChannel
}

// ---------- Alert Rules CRUD ----------

func (s *AlertStore) CreateRule(rule *model.AlertRule) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO alert_rules (name, type, target_id, operator, threshold, unit, duration, level, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.Name, rule.Type, rule.TargetID, rule.Operator, rule.Threshold,
		rule.Unit, rule.Duration, rule.Level, rule.Enabled)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *AlertStore) ListRules() ([]model.AlertRule, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, COALESCE(target_id,''), COALESCE(operator,'>'), threshold,
		        COALESCE(unit,'%'), COALESCE(duration,3), COALESCE(level,'warning'), enabled, created_at
		FROM alert_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.AlertRule
	for rows.Next() {
		var r model.AlertRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.TargetID, &r.Operator, &r.Threshold,
			&r.Unit, &r.Duration, &r.Level, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *AlertStore) ListEnabledRules() ([]model.AlertRule, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, COALESCE(target_id,''), COALESCE(operator,'>'), threshold,
		        COALESCE(unit,'%'), COALESCE(duration,3), COALESCE(level,'warning'), enabled, created_at
		FROM alert_rules WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.AlertRule
	for rows.Next() {
		var r model.AlertRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.TargetID, &r.Operator, &r.Threshold,
			&r.Unit, &r.Duration, &r.Level, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *AlertStore) UpdateRule(rule *model.AlertRule) error {
	_, err := s.db.Exec(
		`UPDATE alert_rules SET name=?, type=?, target_id=?, operator=?, threshold=?, unit=?, duration=?, level=?, enabled=? WHERE id=?`,
		rule.Name, rule.Type, rule.TargetID, rule.Operator, rule.Threshold,
		rule.Unit, rule.Duration, rule.Level, rule.Enabled, rule.ID)
	return err
}

func (s *AlertStore) DeleteRule(id int) error {
	_, err := s.db.Exec("DELETE FROM alert_rules WHERE id=?", id)
	return err
}

func (s *AlertStore) SetRuleEnabled(id int, enabled bool) error {
	_, err := s.db.Exec("UPDATE alert_rules SET enabled=? WHERE id=?", enabled, id)
	return err
}

// ---------- Notification Channels CRUD ----------

func (s *AlertStore) CreateChannel(ch *model.NotificationChannel) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO notification_channels (name, type, config, enabled)
		VALUES (?, ?, ?, ?)`,
		ch.Name, ch.Type, ch.Config, ch.Enabled)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *AlertStore) ListChannels() ([]model.NotificationChannel, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, config, enabled, created_at FROM notification_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []model.NotificationChannel
	for rows.Next() {
		var ch model.NotificationChannel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &ch.Enabled, &ch.CreatedAt); err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, nil
}

func (s *AlertStore) ListEnabledChannels() ([]model.NotificationChannel, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, config, enabled, created_at FROM notification_channels WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []model.NotificationChannel
	for rows.Next() {
		var ch model.NotificationChannel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &ch.Enabled, &ch.CreatedAt); err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, nil
}

func (s *AlertStore) GetChannel(id int) (*model.NotificationChannel, error) {
	var ch model.NotificationChannel
	err := s.db.QueryRow(
		`SELECT id, name, type, config, enabled, created_at FROM notification_channels WHERE id=?`, id).
		Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &ch.Enabled, &ch.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func (s *AlertStore) UpdateChannel(ch *model.NotificationChannel) error {
	_, err := s.db.Exec(
		`UPDATE notification_channels SET name=?, type=?, config=?, enabled=? WHERE id=?`,
		ch.Name, ch.Type, ch.Config, ch.Enabled, ch.ID)
	return err
}

func (s *AlertStore) DeleteChannel(id int) error {
	_, err := s.db.Exec("DELETE FROM notification_channels WHERE id=?", id)
	return err
}

// FiringAlertTarget holds minimal info for Hub alert target mapping.
type FiringAlertTarget struct {
	EventID  int
	RuleType string
	TargetID string
}

// ListFiringAlertTargets returns event_id, rule_type, target_id for all firing alerts.
func (s *AlertStore) ListFiringAlertTargets() ([]FiringAlertTarget, error) {
	rows, err := s.db.Query(
		`SELECT e.id, r.type, e.target_id
		FROM alert_events e
		JOIN alert_rules r ON r.id = e.rule_id
		WHERE e.status = 'firing'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []FiringAlertTarget
	for rows.Next() {
		var t FiringAlertTarget
		if err := rows.Scan(&t.EventID, &t.RuleType, &t.TargetID); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// ---------- Alert Events ----------

func (s *AlertStore) ListFiringEvents() ([]model.AlertEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, rule_id, rule_name, target_id, COALESCE(target_label,''), level, status,
		        silenced, COALESCE(value,0), COALESCE(message,''), fired_at,
		        resolved_at, COALESCE(resolve_type,''), acked_at, COALESCE(acked_by,'')
		FROM alert_events WHERE status='firing' ORDER BY fired_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *AlertStore) QueryEvents(status string, silenced *bool, since, until string, limit, offset int) ([]model.AlertEvent, error) {
	var where []string
	var args []interface{}

	if status != "" {
		where = append(where, "status=?")
		args = append(args, status)
	}
	if silenced != nil {
		where = append(where, "silenced=?")
		args = append(args, *silenced)
	}
	if since != "" {
		where = append(where, "fired_at >= ?")
		args = append(args, since)
	}
	if until != "" {
		where = append(where, "fired_at <= ?")
		args = append(args, until)
	}

	query := `SELECT id, rule_id, rule_name, target_id, COALESCE(target_label,''), level, status,
	                 silenced, COALESCE(value,0), COALESCE(message,''), fired_at,
	                 resolved_at, COALESCE(resolve_type,''), acked_at, COALESCE(acked_by,'')
	          FROM alert_events`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY fired_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *AlertStore) GetStats() (*model.AlertStats, error) {
	var st model.AlertStats
	err := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN status='firing' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='firing' AND silenced=0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='firing' AND fired_at >= date('now','start of day') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='resolved' AND resolved_at >= date('now','start of day') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN silenced=1 AND acked_at >= date('now','start of day') THEN 1 ELSE 0 END), 0)
		FROM alert_events
	`).Scan(&st.Firing, &st.FiringUnsilenced, &st.TodayFired, &st.TodayResolved, &st.TodaySilenced)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *AlertStore) AckEvent(id int, username string) error {
	_, err := s.db.Exec(
		`UPDATE alert_events SET silenced=1, acked_at=?, acked_by=? WHERE id=?`,
		time.Now(), username, id)
	return err
}

func (s *AlertStore) ResolveEventsByRule(ruleID int, resolveType string) error {
	_, err := s.db.Exec(
		`UPDATE alert_events SET status='resolved', resolved_at=?, resolve_type=? WHERE rule_id=? AND status='firing'`,
		time.Now(), resolveType, ruleID)
	return err
}

// ---------- Transactional FireAlert ----------

func (s *AlertStore) FireAlert(event *model.AlertEvent) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 1. Insert alert event
	result, err := tx.Exec(
		`INSERT INTO alert_events (rule_id, rule_name, target_id, target_label, level, status, silenced, value, message, fired_at)
		VALUES (?, ?, ?, ?, ?, 'firing', 0, ?, ?, ?)`,
		event.RuleID, event.RuleName, event.TargetID, event.TargetLabel,
		event.Level, event.Value, event.Message, event.FiredAt)
	if err != nil {
		return 0, err
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// 2. Query all enabled notification channels
	rows, err := tx.Query(`SELECT id FROM notification_channels WHERE enabled=1`)
	if err != nil {
		return 0, err
	}
	var channelIDs []int
	for rows.Next() {
		var cid int
		if err := rows.Scan(&cid); err != nil {
			rows.Close()
			return 0, err
		}
		channelIDs = append(channelIDs, cid)
	}
	rows.Close()

	// 3. Insert notification per channel
	for _, cid := range channelIDs {
		_, err := tx.Exec(
			`INSERT INTO alert_notifications (event_id, channel_id, notify_type, status)
			VALUES (?, ?, 'firing', 'pending')`,
			eventID, cid)
		if err != nil {
			return 0, err
		}
	}

	// 4. Commit
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return eventID, nil
}

// ---------- Transactional ResolveAlert ----------

func (s *AlertStore) ResolveAlert(eventID int, resolveType string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Update event status
	_, err = tx.Exec(
		`UPDATE alert_events SET status='resolved', resolved_at=?, resolve_type=? WHERE id=?`,
		time.Now(), resolveType, eventID)
	if err != nil {
		return err
	}

	// 2. Only for auto resolve: create resolve notifications for same channels
	if resolveType == "auto" {
		rows, err := tx.Query(
			`SELECT DISTINCT channel_id FROM alert_notifications WHERE event_id=? AND notify_type='firing'`,
			eventID)
		if err != nil {
			return err
		}
		var channelIDs []int
		for rows.Next() {
			var cid int
			if err := rows.Scan(&cid); err != nil {
				rows.Close()
				return err
			}
			channelIDs = append(channelIDs, cid)
		}
		rows.Close()

		for _, cid := range channelIDs {
			_, err := tx.Exec(
				`INSERT INTO alert_notifications (event_id, channel_id, notify_type, status)
				VALUES (?, ?, 'resolved', 'pending')`,
				eventID, cid)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// ---------- Notification Delivery ----------

func (s *AlertStore) ClaimNotification(id int) (bool, error) {
	result, err := s.db.Exec(
		`UPDATE alert_notifications SET status='sending', claimed_at=? WHERE id=? AND status='pending'`,
		time.Now(), id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *AlertStore) MarkNotificationSent(id int) error {
	_, err := s.db.Exec(
		`UPDATE alert_notifications SET status='sent', sent_at=? WHERE id=?`,
		time.Now(), id)
	return err
}

func (s *AlertStore) MarkNotificationFailed(id int, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE alert_notifications SET
			retry_count = retry_count + 1,
			last_error = ?,
			status = CASE WHEN retry_count + 1 < 3 THEN 'pending' ELSE 'failed' END
		WHERE id=?`,
		errMsg, id)
	return err
}

func (s *AlertStore) ListPendingNotifications() ([]PendingNotification, error) {
	rows, err := s.db.Query(`
		SELECT
			n.id, n.notify_type,
			e.id, e.rule_id, e.rule_name, e.target_id, COALESCE(e.target_label,''), e.level, e.status,
			e.silenced, COALESCE(e.value,0), COALESCE(e.message,''), e.fired_at,
			e.resolved_at, COALESCE(e.resolve_type,''), e.acked_at, COALESCE(e.acked_by,''),
			c.id, c.name, c.type, c.config, c.enabled, c.created_at
		FROM alert_notifications n
		JOIN alert_events e ON e.id = n.event_id
		JOIN notification_channels c ON c.id = n.channel_id
		WHERE n.status='pending'
		ORDER BY n.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PendingNotification
	for rows.Next() {
		var p PendingNotification
		var resolvedAt, ackedAt sql.NullTime
		if err := rows.Scan(
			&p.ID, &p.NotifyType,
			&p.Event.ID, &p.Event.RuleID, &p.Event.RuleName, &p.Event.TargetID, &p.Event.TargetLabel,
			&p.Event.Level, &p.Event.Status, &p.Event.Silenced, &p.Event.Value, &p.Event.Message,
			&p.Event.FiredAt, &resolvedAt, &p.Event.ResolveType, &ackedAt, &p.Event.AckedBy,
			&p.Channel.ID, &p.Channel.Name, &p.Channel.Type, &p.Channel.Config, &p.Channel.Enabled, &p.Channel.CreatedAt,
		); err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			p.Event.ResolvedAt = &resolvedAt.Time
		}
		if ackedAt.Valid {
			p.Event.AckedAt = &ackedAt.Time
		}
		list = append(list, p)
	}
	return list, nil
}

func (s *AlertStore) ResetStaleNotifications() error {
	// Reset 'sending' notifications that have been claimed for over 60 seconds (likely stuck)
	_, err := s.db.Exec(
		`UPDATE alert_notifications SET status='pending', claimed_at=NULL
		 WHERE status='sending' AND claimed_at < datetime('now', '-60 seconds')`)
	return err
}

// ResetAllSendingNotifications resets ALL sending notifications to pending (used on startup only).
func (s *AlertStore) ResetAllSendingNotifications() error {
	_, err := s.db.Exec(
		`UPDATE alert_notifications SET status='pending', claimed_at=NULL WHERE status='sending'`)
	return err
}

func (s *AlertStore) GetEventNotifications(eventID int) ([]model.AlertNotificationDetail, error) {
	rows, err := s.db.Query(`
		SELECT c.name, c.type, n.notify_type, n.status, n.retry_count,
		       COALESCE(n.last_error,''), n.sent_at
		FROM alert_notifications n
		JOIN notification_channels c ON c.id = n.channel_id
		WHERE n.event_id=?
		ORDER BY n.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.AlertNotificationDetail
	for rows.Next() {
		var d model.AlertNotificationDetail
		var sentAt sql.NullTime
		if err := rows.Scan(&d.ChannelName, &d.ChannelType, &d.NotifyType, &d.Status,
			&d.RetryCount, &d.LastError, &sentAt); err != nil {
			return nil, err
		}
		if sentAt.Valid {
			d.SentAt = &sentAt.Time
		}
		list = append(list, d)
	}
	return list, nil
}

// ---------- helpers ----------

func scanEvents(rows *sql.Rows) ([]model.AlertEvent, error) {
	var events []model.AlertEvent
	for rows.Next() {
		var e model.AlertEvent
		var resolvedAt, ackedAt sql.NullTime
		if err := rows.Scan(
			&e.ID, &e.RuleID, &e.RuleName, &e.TargetID, &e.TargetLabel,
			&e.Level, &e.Status, &e.Silenced, &e.Value, &e.Message,
			&e.FiredAt, &resolvedAt, &e.ResolveType, &ackedAt, &e.AckedBy,
		); err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			e.ResolvedAt = &resolvedAt.Time
		}
		if ackedAt.Valid {
			e.AckedAt = &ackedAt.Time
		}
		events = append(events, e)
	}
	return events, nil
}

