package storage

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/common"

	"github.com/matrix-org/gomatrixserverlib"
)

const inviteEventsSchema = `
CREATE TABLE IF NOT EXISTS syncapi_invite_events (
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_stream_id'),
	event_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	target_user_id TEXT NOT NULL,
	event_json TEXT NOT NULL,
);

-- For looking up the invites for a given user.
CREATE INDEX IF NOT EXISTS syncapi_target_user_id_idx
	ON syncapi_invite_events (target_user_id, id)
);
`

const insertInviteEventSQL = "" +
	"INSERT INTO syncapi_invite_events (" +
	" room_id, event_id, target_user_id, event_json" +
	") VALUES ($1, $2, $3, $4) RETURNING id"

const selectInviteEventsInRangeSQL = "" +
	"SELECT room_id, event_json FROM syncapi_invite_events" +
	" WHERE target_user_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id DESC"

type inviteEventsStatements struct {
	insertInviteEventStmt         *sql.Stmt
	selectInviteEventsInRangeStmt *sql.Stmt
}

func (s *inviteEventsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(inviteEventsSchema)
	if err != nil {
		return
	}
	if s.insertInviteEventStmt, err = db.Prepare(insertInviteEventSQL); err != nil {
		return
	}
	if s.selectInviteEventsInRangeStmt, err = db.Prepare(selectInviteEventsInRangeSQL); err != nil {
		return
	}
	return
}

func (s *inviteEventsStatements) insertInviteEvent(
	ctx context.Context, inviteEvent gomatrixserverlib.Event,
) (streamPos int64, err error) {
	err = s.insertInviteEventStmt.QueryRowContext(
		ctx,
		inviteEvent.RoomID(),
		inviteEvent.EventID(),
		*inviteEvent.StateKey(),
		inviteEvent.JSON(),
	).Scan(&streamPos)
	return
}

// selectInviteEventsInRange returns a map of room ID to invite event for the
// active invites for the target user ID in the supplied range.
func (s *inviteEventsStatements) selectInviteEventsInRange(
	ctx context.Context, txn *sql.Tx, targetUserID string, startPos, endPos int64,
) (map[string]gomatrixserverlib.Event, error) {
	stmt := common.TxStmt(txn, s.selectInviteEventsInRangeStmt)
	rows, err := stmt.QueryContext(ctx, targetUserID, startPos, endPos)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	result := map[string]gomatrixserverlib.Event{}
	for rows.Next() {
		var (
			roomID    string
			eventJSON []byte
		)
		if err = rows.Scan(&roomID, &eventJSON); err != nil {
			return nil, err
		}

		event, err := gomatrixserverlib.NewEventFromTrustedJSON(eventJSON, false)
		if err != nil {
			return nil, err
		}

		result[roomID] = event
	}
	return result, nil
}
