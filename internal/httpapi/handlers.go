package httpapi

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

// --- auth & me ---

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	key := domain.NormalizeUsername(in.Username)
	if ok, wait := a.limiter.Allowed(key); !ok {
		w.Header().Set("Retry-After", fmt.Sprint(int(math.Ceil(wait.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many failed login attempts, try again later")
		return
	}
	secret, u, err := a.svc.Login(r.Context(), in.Username, in.Password, r.UserAgent())
	if err != nil {
		if domainKind(err) == domain.KindUnauthorized {
			a.limiter.Failure(key)
		}
		respondErr(w, r, a.log, err)
		return
	}
	a.limiter.Success(key)
	a.setSessionCookie(w, r, secret)
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Logout(r.Context(), principal(r)); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	a.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

type meResponse struct {
	User *domain.User `json:"user"`
	Auth struct {
		Type     string        `json:"type"` // session | token
		Scopes   domain.Scopes `json:"scopes"`
		TokenID  string        `json:"token_id,omitempty"`
		TopicIDs []string      `json:"topic_ids,omitempty"`
		// HasPassword tells the UI whether a password change makes sense.
		HasPassword bool `json:"has_password"`
	} `json:"auth"`
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var resp meResponse
	resp.User, resp.Auth.Type, resp.Auth.Scopes = p.User, "session", p.Scopes
	resp.Auth.HasPassword = p.User.PasswordHash != ""
	if p.Token != nil {
		resp.Auth.Type, resp.Auth.TokenID, resp.Auth.TopicIDs = "token", p.Token.ID, p.Token.TopicIDs
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *api) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	u, err := a.svc.UpdateProfile(r.Context(), principal(r), in.DisplayName, in.Email)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *api) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	if err := a.svc.ChangePassword(r.Context(), principal(r), in.Current, in.New); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- users (admin) ---

func (a *api) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.svc.ListUsers(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

func (a *api) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string      `json:"username"`
		Password    string      `json:"password"`
		DisplayName string      `json:"display_name"`
		Email       string      `json:"email"`
		Role        domain.Role `json:"role"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	u, err := a.svc.CreateUser(r.Context(), principal(r), service.CreateUserInput{
		Username: in.Username, Password: in.Password, DisplayName: in.DisplayName, Email: in.Email, Role: in.Role,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (a *api) getUser(w http.ResponseWriter, r *http.Request) {
	u, err := a.svc.GetUser(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *api) updateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName *string      `json:"display_name"`
		Email       *string      `json:"email"`
		Role        *domain.Role `json:"role"`
		Disabled    *bool        `json:"disabled"`
		Password    *string      `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	u, err := a.svc.UpdateUser(r.Context(), principal(r), chi.URLParam(r, "id"), service.UpdateUserInput{
		DisplayName: in.DisplayName, Email: in.Email, Role: in.Role, Disabled: in.Disabled, Password: in.Password,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *api) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteUser(r.Context(), principal(r), chi.URLParam(r, "id")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- topics ---

func (a *api) listTopics(w http.ResponseWriter, r *http.Request) {
	topics, err := a.svc.ListTopics(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": topics})
}

func (a *api) createTopic(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Color           *string `json:"color"`
		Name            string  `json:"name"`
		Title           string  `json:"title"`
		Description     string  `json:"description"`
		DefaultPriority *int    `json:"default_priority"`
		RetentionDays   *int    `json:"retention_days"`
		MaxCount        *int    `json:"max_notifications"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	t, err := a.svc.CreateTopic(r.Context(), principal(r), service.CreateTopicInput{
		Color: in.Color,
		Name:  in.Name, Title: in.Title, Description: in.Description, DefaultPriority: in.DefaultPriority,
		RetentionDays: in.RetentionDays, MaxNotifications: in.MaxCount,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (a *api) getTopic(w http.ResponseWriter, r *http.Request) {
	t, err := a.svc.GetTopic(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *api) updateTopic(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Color           *string  `json:"color"`
		Title           *string  `json:"title"`
		Description     *string  `json:"description"`
		DefaultPriority opt[int] `json:"default_priority"`
		RetentionDays   opt[int] `json:"retention_days"`
		MaxCount        opt[int] `json:"max_notifications"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	t, err := a.svc.UpdateTopic(r.Context(), principal(r), chi.URLParam(r, "id"), service.UpdateTopicInput{
		Color: in.Color,
		Title: in.Title, Description: in.Description,
		DefaultPriority: in.DefaultPriority.Value, SetDefaultPriority: in.DefaultPriority.Set,
		RetentionDays: in.RetentionDays.Value, SetRetentionDays: in.RetentionDays.Set,
		MaxNotifications: in.MaxCount.Value, SetMaxNotifications: in.MaxCount.Set,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *api) deleteTopic(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteTopic(r.Context(), principal(r), chi.URLParam(r, "id")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) listTopicMembers(w http.ResponseWriter, r *http.Request) {
	members, err := a.svc.ListTopicMembers(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": members})
}

func (a *api) listTopicInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := a.svc.ListTopicInvites(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": invites})
}

func (a *api) inviteToTopic(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	inv, err := a.svc.InviteToTopic(r.Context(), principal(r), chi.URLParam(r, "id"), in.Username)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (a *api) revokeTopicInvite(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.RevokeTopicInvite(r.Context(), principal(r), chi.URLParam(r, "id"), chi.URLParam(r, "userID")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) listInvitations(w http.ResponseWriter, r *http.Request) {
	invites, err := a.svc.ListMyInvites(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": invites})
}

func (a *api) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	t, err := a.svc.AcceptTopicInvite(r.Context(), principal(r), chi.URLParam(r, "topicID"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *api) declineInvitation(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeclineTopicInvite(r.Context(), principal(r), chi.URLParam(r, "topicID")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) removeTopicMember(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.RemoveTopicMember(r.Context(), principal(r), chi.URLParam(r, "id"), chi.URLParam(r, "userID")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- tokens ---

func (a *api) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := a.svc.ListTokens(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tokens})
}

func (a *api) createToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		Topics    []string   `json:"topics"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	ct, err := a.svc.CreateToken(r.Context(), principal(r), service.CreateTokenInput{
		Name: in.Name, Scopes: in.Scopes, Topics: in.Topics, ExpiresAt: in.ExpiresAt,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": ct.Token, "secret": ct.Secret})
}

func (a *api) deleteToken(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteToken(r.Context(), principal(r), chi.URLParam(r, "id")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- notifications ---

type publishRequest struct {
	Topic     string          `json:"topic"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Priority  int             `json:"priority"`
	Tags      []string        `json:"tags"`
	ClickURL  string          `json:"click_url"`
	Actions   []domain.Action `json:"actions"`
	GroupKey  string          `json:"group_key"`
	DedupKey  string          `json:"dedup_key"`
	ExpiresAt *time.Time      `json:"expires_at"`
}

func (a *api) publish(w http.ResponseWriter, r *http.Request) {
	var in publishRequest
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	res, err := a.svc.Publish(r.Context(), principal(r), domain.Message{
		Topic: in.Topic, Title: in.Title, Body: in.Body, Priority: in.Priority, Tags: in.Tags,
		ClickURL: in.ClickURL, Actions: in.Actions, GroupKey: in.GroupKey, DedupKey: in.DedupKey,
		ExpiresAt: in.ExpiresAt, Source: "acksy",
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (a *api) listNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lq := service.ListQuery{
		View: domain.View(q.Get("view")), Topic: q.Get("topic"), GroupKey: q.Get("group_key"), Before: q.Get("before"),
		Search: q.Get("q"),
	}
	var err error
	if lq.UnreadOnly, err = queryBool(r, "unread"); err == nil {
		if lq.NoTopic, err = queryBool(r, "no_topic"); err == nil {
			if lq.MinPriority, err = queryInt(r, "min_priority"); err == nil {
				lq.Limit, err = queryInt(r, "limit")
			}
		}
	}
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	page, err := a.svc.ListNotifications(r.Context(), principal(r), lq)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *api) getNotification(w http.ResponseWriter, r *http.Request) {
	n, err := a.svc.GetNotification(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *api) updateNotification(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Read         *bool          `json:"read"`
		Archived     *bool          `json:"archived"`
		SnoozedUntil opt[time.Time] `json:"snoozed_until"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	n, err := a.svc.UpdateNotificationState(r.Context(), principal(r), chi.URLParam(r, "id"), service.StatePatch{
		Read: in.Read, Archived: in.Archived, SnoozedUntil: in.SnoozedUntil.Value, SetSnooze: in.SnoozedUntil.Set,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *api) deleteNotification(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteNotification(r.Context(), principal(r), chi.URLParam(r, "id")); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) bulkNotifications(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action   domain.BulkAction `json:"action"`
		IDs      []string          `json:"ids"`
		Topic    string            `json:"topic"`
		NoTopic  bool              `json:"no_topic"`
		GroupKey string            `json:"group_key"`
		View     domain.View       `json:"view"`
		All      bool              `json:"all"`
		Q        string            `json:"q"`
		MinPrio  int               `json:"min_priority"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	if in.View != "" && !in.View.Valid() {
		respondErr(w, r, a.log, domain.Invalid("view must be inbox, archived, snoozed or all"))
		return
	}
	res, err := a.svc.Bulk(r.Context(), principal(r), in.Action, service.BulkSelector{
		IDs: in.IDs, Topic: in.Topic, NoTopic: in.NoTopic, GroupKey: in.GroupKey, View: in.View, All: in.All,
		Search: in.Q, MinPriority: in.MinPrio,
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *api) counts(w http.ResponseWriter, r *http.Request) {
	c, err := a.svc.Counts(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func domainKind(err error) domain.Kind {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Kind
	}
	return 0
}

func (a *api) runAction(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(chi.URLParam(r, "index"))
	if err != nil {
		respondErr(w, r, a.log, domain.NotFound("action"))
		return
	}
	res, err := a.svc.RunAction(r.Context(), principal(r), chi.URLParam(r, "id"), index)
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
