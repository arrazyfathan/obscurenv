package handlers

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

type activityMetadataMatcher struct {
	direction    string
	counterparty string
}

func (matcher activityMetadataMatcher) Match(value driver.Value) bool {
	data, ok := value.([]byte)
	if !ok {
		return false
	}
	var metadata map[string]string
	if json.Unmarshal(data, &metadata) != nil {
		return false
	}
	return metadata["direction"] == matcher.direction && metadata["counterparty"] == matcher.counterparty
}

func TestActivityAccountLabelUsesSafeDisplayIdentity(t *testing.T) {
	tests := []struct {
		name     string
		username sql.NullString
		email    string
		want     string
	}{
		{name: "username", username: sql.NullString{String: "alice", Valid: true}, email: "alice@example.com", want: "@alice"},
		{name: "email fallback", username: sql.NullString{}, email: "alice@example.com", want: "a***@example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := activityAccountLabel(test.username, test.email); got != test.want {
				t.Fatalf("activityAccountLabel() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTransferAccountLabelUsesSafeDisplayIdentity(t *testing.T) {
	account := transferAccount{Username: sql.NullString{}, Email: "alice@example.com"}
	if got := account.Label(); got != "a***@example.com" {
		t.Fatalf("transferAccount.Label() = %q, want masked email", got)
	}
}

func TestCreateShareInvitationRecordsBothPerspectives(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	expectProjectAccess(mock, "owner-1", "app", "project-1", "owner-1", "owner")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, username, email FROM users").WithArgs("user-2").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email"}).AddRow("user-2", "alice", "alice@example.com"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("user-2", "app", "project-1").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("INSERT INTO project_share_invitations").WithArgs("project-1", "owner-1", "user-2", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("invite-1"))
	mock.ExpectQuery("SELECT id, username, email FROM users WHERE id").WithArgs("owner-1").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email"}).AddRow("owner-1", "owner", "owner@example.com"))
	expectActivity(mock, "owner-1", "project-1", ActionShareInvited, "app", "outgoing", "@alice")
	expectActivity(mock, "user-2", "project-1", ActionShareInvited, "app", "incoming", "@owner")
	mock.ExpectCommit()

	handler := NewSharingHandler(db)
	router := sharingTestRouter("owner-1")
	router.POST("/projects/:slug/share-invitations", handler.CreateInvitation)
	req := httptest.NewRequest(http.MethodPost, "/projects/app/share-invitations", strings.NewReader(`{"recipient_user_id":"user-2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateInvitation returned %d: %s", rec.Code, rec.Body.String())
	}
	assertExpectations(t, mock)
}

func TestResolveShareInvitationRecordsBothPerspectives(t *testing.T) {
	for _, test := range []struct {
		name   string
		result string
		action string
	}{
		{name: "accepted", result: shareAccepted, action: ActionShareAccepted},
		{name: "declined", result: shareDeclined, action: ActionShareDeclined},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT si.status, si.project_id").WithArgs("invite-1", "user-2").WillReturnRows(sqlmock.NewRows([]string{"status", "project_id", "slug", "sender_user_id", "recipient_user_id", "expires_at", "sender_username", "sender_email", "recipient_username", "recipient_email"}).AddRow(sharePending, "project-1", "app", "owner-1", "user-2", time.Now().Add(time.Hour), "owner", "owner@example.com", "alice", "alice@example.com"))
			if test.result == shareAccepted {
				mock.ExpectQuery("SELECT EXISTS").WithArgs("user-2", "app", "project-1").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
				mock.ExpectExec("INSERT INTO project_members").WithArgs("project-1", "user-2", "owner-1").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec("UPDATE project_share_invitations SET status").WithArgs("invite-1", test.result).WillReturnResult(sqlmock.NewResult(0, 1))
			expectActivity(mock, "user-2", "project-1", test.action, "app", "incoming", "@owner")
			expectActivity(mock, "owner-1", "project-1", test.action, "app", "outgoing", "@alice")
			mock.ExpectCommit()

			handler := NewSharingHandler(db)
			router := sharingTestRouter("user-2")
			if test.result == shareAccepted {
				router.POST("/project-share-invitations/:id", handler.Accept)
			} else {
				router.POST("/project-share-invitations/:id", handler.Decline)
			}
			req := httptest.NewRequest(http.MethodPost, "/project-share-invitations/invite-1", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("resolveInvitation returned %d: %s", rec.Code, rec.Body.String())
			}
			assertExpectations(t, mock)
		})
	}
}

func TestCancelShareInvitationRecordsBothPerspectives(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT si.project_id, p.slug").WithArgs("invite-1", "owner-1").WillReturnRows(sqlmock.NewRows([]string{"project_id", "slug", "recipient_user_id", "recipient_username", "recipient_email", "owner_username", "owner_email"}).AddRow("project-1", "app", "user-2", "alice", "alice@example.com", "owner", "owner@example.com"))
	mock.ExpectExec("UPDATE project_share_invitations SET status").WithArgs("invite-1").WillReturnResult(sqlmock.NewResult(0, 1))
	expectActivity(mock, "owner-1", "project-1", ActionShareCanceled, "app", "outgoing", "@alice")
	expectActivity(mock, "user-2", "project-1", ActionShareCanceled, "app", "incoming", "@owner")
	mock.ExpectCommit()

	handler := NewSharingHandler(db)
	router := sharingTestRouter("owner-1")
	router.DELETE("/project-share-invitations/:id", handler.Cancel)
	req := httptest.NewRequest(http.MethodDelete, "/project-share-invitations/invite-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Cancel returned %d: %s", rec.Code, rec.Body.String())
	}
	assertExpectations(t, mock)
}

func TestCancelShareInvitationRollsBackWhenMirroredActivityFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT si.project_id, p.slug").WithArgs("invite-1", "owner-1").WillReturnRows(sqlmock.NewRows([]string{"project_id", "slug", "recipient_user_id", "recipient_username", "recipient_email", "owner_username", "owner_email"}).AddRow("project-1", "app", "user-2", "alice", "alice@example.com", "owner", "owner@example.com"))
	mock.ExpectExec("UPDATE project_share_invitations SET status").WithArgs("invite-1").WillReturnResult(sqlmock.NewResult(0, 1))
	expectActivity(mock, "owner-1", "project-1", ActionShareCanceled, "app", "outgoing", "@alice")
	mock.ExpectExec("INSERT INTO activity_logs").WithArgs("user-2", "project-1", ActionShareCanceled, "app", nil, activityMetadataMatcher{direction: "incoming", counterparty: "@owner"}).WillReturnError(errors.New("activity write failed"))
	mock.ExpectRollback()

	handler := NewSharingHandler(db)
	router := sharingTestRouter("owner-1")
	router.DELETE("/project-share-invitations/:id", handler.Cancel)
	req := httptest.NewRequest(http.MethodDelete, "/project-share-invitations/invite-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Cancel returned %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	assertExpectations(t, mock)
}

func TestRemoveMemberRecordsBothPerspectives(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	expectProjectAccess(mock, "owner-1", "app", "project-1", "owner-1", "owner")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT member.username, member.email").WithArgs("project-1", "user-2", "owner-1").WillReturnRows(sqlmock.NewRows([]string{"member_username", "member_email", "owner_username", "owner_email"}).AddRow("alice", "alice@example.com", "owner", "owner@example.com"))
	mock.ExpectExec("DELETE FROM project_members").WithArgs("project-1", "user-2").WillReturnResult(sqlmock.NewResult(0, 1))
	expectActivity(mock, "owner-1", "project-1", ActionShareRemoved, "app", "outgoing", "@alice")
	expectActivity(mock, "user-2", "project-1", ActionShareRemoved, "app", "incoming", "@owner")
	mock.ExpectCommit()

	handler := NewSharingHandler(db)
	router := sharingTestRouter("owner-1")
	router.DELETE("/projects/:slug/members/:user_id", handler.RemoveMember)
	req := httptest.NewRequest(http.MethodDelete, "/projects/app/members/user-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("RemoveMember returned %d: %s", rec.Code, rec.Body.String())
	}
	assertExpectations(t, mock)
}

func TestLeaveProjectRecordsBothPerspectives(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	expectProjectAccess(mock, "user-2", "app", "project-1", "owner-1", "collaborator")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT member.username, member.email").WithArgs("project-1", "user-2", "owner-1").WillReturnRows(sqlmock.NewRows([]string{"member_username", "member_email", "owner_username", "owner_email"}).AddRow("alice", "alice@example.com", "owner", "owner@example.com"))
	mock.ExpectExec("DELETE FROM project_members").WithArgs("project-1", "user-2").WillReturnResult(sqlmock.NewResult(0, 1))
	expectActivity(mock, "user-2", "project-1", ActionShareLeft, "app", "incoming", "@owner")
	expectActivity(mock, "owner-1", "project-1", ActionShareLeft, "app", "outgoing", "@alice")
	mock.ExpectCommit()

	handler := NewSharingHandler(db)
	router := sharingTestRouter("user-2")
	router.DELETE("/projects/:slug/membership", handler.Leave)
	req := httptest.NewRequest(http.MethodDelete, "/projects/app/membership", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Leave returned %d: %s", rec.Code, rec.Body.String())
	}
	assertExpectations(t, mock)
}

func expectProjectAccess(mock sqlmock.Sqlmock, userID, slug, projectID, ownerID, role string) {
	mock.ExpectQuery("SELECT p.id, p.user_id").WithArgs(userID, slug).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "role"}).AddRow(projectID, ownerID, role))
}

func expectActivity(mock sqlmock.Sqlmock, userID, projectID, action, slug, direction, counterparty string) {
	mock.ExpectExec("INSERT INTO activity_logs").WithArgs(userID, projectID, action, slug, nil, activityMetadataMatcher{direction: direction, counterparty: counterparty}).WillReturnResult(sqlmock.NewResult(0, 1))
}

func sharingTestRouter(userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	})
	return router
}

func assertExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
