package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rss2go/internal/crawler"
	"rss2go/internal/database"
	"rss2go/internal/logger"
	"rss2go/internal/types"
)



type unsubscribeRequest struct {
	Email   string  `json:"email"`
	Token   string  `json:"token"`
	FeedIDs []int64 `json:"feed_ids"`
}

type subscriberManageResponse struct {
	Email string           `json:"email"`
	Token string           `json:"token"`
	Feeds []subscriberFeed `json:"feeds"`
}

type subscriberFeed struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Subscribed bool   `json:"subscribed"`
}

type subscriptionPayload struct {
	UserID int64 `json:"user_id"`
	FeedID int64 `json:"feed_id"`
}

type rewindPayload struct {
	Limit int `json:"limit"`
}

type testFeedResponse struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Items       []testFeedItem `json:"items"`
}

type testFeedItem struct {
	Title            string `json:"title"`
	Link             string `json:"link"`
	GUID             string `json:"guid"`
	Content          string `json:"content"`
	ExtractedContent string `json:"extracted_content,omitempty"`
}

// JSON formatting utilities
func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}



// handleSubscriberManage verifies public magic tokens and returns subscription preferences.
func (s *Server) handleSubscriberManage(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	token := r.URL.Query().Get("token")

	if email == "" || token == "" {
		s.writeError(w, http.StatusBadRequest, "Missing email or token parameters")
		return
	}

	if !verifyMagicToken(email, token, s.cfg.MagicSecret) {
		s.writeError(w, http.StatusForbidden, "Invalid verification token")
		return
	}

	user, err := s.repo.GetUserByEmail(r.Context(), email)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Subscriber profile not found")
		return
	}

	feeds, err := s.repo.ListFeeds(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	subscribedFeeds, err := s.repo.ListSubscriptionsForUser(r.Context(), user.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	subMap := make(map[int64]bool)
	for _, f := range subscribedFeeds {
		subMap[f.ID] = true
	}

	var res subscriberManageResponse
	res.Email = email
	res.Token = token
	for _, f := range feeds {
		res.Feeds = append(res.Feeds, subscriberFeed{
			ID:         f.ID,
			Title:      f.Title,
			Subscribed: subMap[f.ID],
		})
	}

	s.writeJSON(w, http.StatusOK, res)
}

// handleSubscriberUnsubscribe unsubscribes/resubscribes a public subscriber atomically.
func (s *Server) handleSubscriberUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req unsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if !verifyMagicToken(req.Email, req.Token, s.cfg.MagicSecret) {
		s.writeError(w, http.StatusForbidden, "Invalid verification token")
		return
	}

	user, err := s.repo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Subscriber profile not found")
		return
	}

	// Update preferences atomically inside a transaction
	txErr := s.repo.WithTx(r.Context(), func(txRepo *database.Repository) error {
		currentSubs, err := txRepo.ListSubscriptionsForUser(r.Context(), user.ID)
		if err != nil {
			return err
		}

		targetMap := make(map[int64]bool)
		for _, fid := range req.FeedIDs {
			targetMap[fid] = true
		}

		// Unsubscribe from removed feeds
		for _, f := range currentSubs {
			if !targetMap[f.ID] {
				if err := txRepo.Unsubscribe(r.Context(), user.ID, f.ID); err != nil {
					return err
				}
			}
		}

		// Subscribe to newly added feeds
		currentMap := make(map[int64]bool)
		for _, f := range currentSubs {
			currentMap[f.ID] = true
		}

		for _, fid := range req.FeedIDs {
			if !currentMap[fid] {
				if err := txRepo.Subscribe(r.Context(), user.ID, fid); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if txErr != nil {
		s.writeError(w, http.StatusInternalServerError, txErr.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"message": "Subscription preferences updated successfully"})
}

// handleGetFeeds lists all configured feeds.
func (s *Server) handleGetFeeds(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("handleGetFeeds: entered handler", "host", r.Host)
	feeds, err := s.repo.ListFeeds(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Debug("handleGetFeeds: returning feeds", "host", r.Host, "count", len(feeds))
	s.writeJSON(w, http.StatusOK, feeds)
}

// handleCreateFeed enqueues a new feed source.
func (s *Server) handleCreateFeed(w http.ResponseWriter, r *http.Request) {
	var feed types.Feed
	if err := json.NewDecoder(r.Body).Decode(&feed); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if feed.Title == "" || feed.URL == "" {
		s.writeError(w, http.StatusBadRequest, "Title and URL are required")
		return
	}

	feed.NextPollAt = time.Now()
	feed.BackoffFactor = 1.0

	if err := s.repo.CreateFeed(r.Context(), &feed); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, feed)
}

// handleGetFeedDetails returns configuration and logs for a single feed.
func (s *Server) handleGetFeedDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	feed, err := s.repo.GetFeed(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Feed not found")
		return
	}

	s.writeJSON(w, http.StatusOK, feed)
}

// handleUpdateFeed replaces feed configurations.
func (s *Server) handleUpdateFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	var feed types.Feed
	if err := json.NewDecoder(r.Body).Decode(&feed); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	feed.ID = id
	if err := s.repo.UpdateFeed(r.Context(), &feed); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, feed)
}

// handleDeleteFeed removes a feed and drops related subscriptions.
func (s *Server) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	s.log.Debug("handleDeleteFeed: received request", "host", r.Host, "id", idStr)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.log.Debug("handleDeleteFeed: invalid id error", "host", r.Host, "err", err)
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	if err := s.repo.DeleteFeed(r.Context(), id); err != nil {
		s.log.Debug("handleDeleteFeed: repo delete failed", "host", r.Host, "err", err)
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.log.Debug("handleDeleteFeed: successfully deleted feed", "host", r.Host, "id", id)
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "Feed deleted successfully"})
}

// handleGetUsers lists all users.
func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("handleGetUsers: entered handler", "host", r.Host)
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Debug("handleGetUsers: returning users", "host", r.Host, "count", len(users))
	for _, u := range users {
		s.log.Debug("  user", "id", u.ID, "email", u.Email)
	}
	s.writeJSON(w, http.StatusOK, users)
}

// handleCreateUser adds a new subscriber user profile.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var user types.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if user.Email == "" {
		s.writeError(w, http.StatusBadRequest, "Email is required")
		return
	}

	if err := s.repo.CreateUser(r.Context(), &user); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, user)
}

// handleDeleteUser deletes a user.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	s.log.Debug("handleDeleteUser: received request", "host", r.Host, "id", idStr)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.log.Debug("handleDeleteUser: invalid id error", "host", r.Host, "err", err)
		s.writeError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := s.repo.DeleteUser(r.Context(), id); err != nil {
		s.log.Debug("handleDeleteUser: repo delete failed", "host", r.Host, "err", err)
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.log.Debug("handleDeleteUser: successfully deleted user", "host", r.Host, "id", id)
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}

// handleSubscribe creates a user subscription mapping.
func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	var payload subscriptionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := s.repo.Subscribe(r.Context(), payload.UserID, payload.FeedID); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"message": "Subscribed successfully"})
}

// handleUnsubscribe removes a subscription mapping.
func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var payload subscriptionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := s.repo.Unsubscribe(r.Context(), payload.UserID, payload.FeedID); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"message": "Unsubscribed successfully"})
}

// handleGetStats serves system counts and outbox queues.
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.repo.GetStats(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := struct {
		*types.DBStats
		MailerMode string `json:"mailer_mode"`
		LogLevel   string `json:"log_level"`
	}{
		DBStats:    stats,
		MailerMode: s.cfg.MailerMode,
		LogLevel:   logger.GetGlobalLevel(),
	}
	s.writeJSON(w, http.StatusOK, response)
}

// handleGetOutbox returns the list of recently queued, pending, or failed emails.
func (s *Server) handleGetOutbox(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}

	items, err := s.repo.ListOutboxItems(r.Context(), limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// For security and privacy in the UI report, we strip the raw email body before transmitting
	for _, item := range items {
		item.Body = ""
	}

	s.writeJSON(w, http.StatusOK, items)
}

// handleGetLogs streams logs as they arrive using Server-Sent Events.
// It accepts an optional ?level= query parameter (debug/info/warn/error; default info)
// to filter lines below that severity. History is replayed to the client on connect.
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	// Upgrade response writer to SSE event stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "Streaming unsupported by client transport")
		return
	}

	// Parse optional ?level= threshold (default info = 1).
	threshold := 1
	switch strings.ToLower(r.URL.Query().Get("level")) {
	case "debug":
		threshold = 0
	case "info":
		threshold = 1
	case "warn":
		threshold = 2
	case "error":
		threshold = 3
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := make(chan string, 100)
	history := s.broadcaster.RegisterWithReplay(ch)
	defer s.broadcaster.Unregister(ch)

	// Replay buffered history before entering the live loop.
	for _, line := range history {
		lvl := lineLevel(line)
		if lvl != -1 && lvl < threshold {
			continue
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(line))
	}
	flusher.Flush()

	// Keep-alive heartbeat ticker to prevent socket closure
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-ch:
			lvl := lineLevel(msg)
			if lvl != -1 && lvl < threshold {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(msg))
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// handleTestFeed triggers a dry-run crawl and article extraction preview.
func (s *Server) handleTestFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	feed, err := s.repo.GetFeed(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Feed not found")
		return
	}

	// For a test feed dry-run, we bypass cache headers to force a fresh retrieval.
	testFeed := *feed
	testFeed.ETag = ""
	testFeed.LastModified = ""

	res, err := s.crawler.Crawl(r.Context(), &testFeed)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Crawl dry-run failed: %v", err))
		return
	}

	if res.NotModified {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"message":      "Feed cache status is up-to-date (Not Modified)",
			"not_modified": true,
		})
		return
	}

	var resp testFeedResponse
	resp.Title = res.Feed.Title
	resp.Description = res.Feed.Description

	// Process first 10 items for dry-run inspection, executing extraction preview on the first item
	for i, item := range res.Feed.Items {
		if i >= 10 {
			break
		}

		link := crawler.ResolveItemLink(item)
		tItem := testFeedItem{
			Title: item.Title,
			Link:  link,
			GUID:  item.GUID,
		}

		content := item.Content
		if content == "" {
			content = item.Description
		}

		// Preview extraction for the first item to let the operator verify structural CSS rules/heuristics
		if i == 0 && feed.ExtractFullArticle && link != "" {
			extracted, err := s.extractor.Extract(r.Context(), link, feed.ExtractionStrategy, feed.CSSSelector)
			if err != nil {
				s.log.Warn("Dry-run article extraction failed", "feed", feed.Title, "link", link, "err", err)
			} else {
				tItem.ExtractedContent = extracted
			}
		}

		sanitized, err := s.sanitizer.Sanitize(content, feed.URL)
		if err != nil {
			s.log.Warn("Dry-run sanitize content failed", "err", err)
			sanitized = content
		}
		tItem.Content = sanitized

		resp.Items = append(resp.Items, tItem)
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleCatchupFeed marks all current crawl items as seen.
func (s *Server) handleCatchupFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	feed, err := s.repo.GetFeed(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Feed not found")
		return
	}

	res, err := s.crawler.Crawl(r.Context(), feed)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Crawl fetch failed during catchup: %v", err))
		return
	}

	if res.NotModified {
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "Feed already up-to-date"})
		return
	}

	var count int
	for _, item := range res.Feed.Items {
		guid := item.GUID
		if guid == "" {
			guid = crawler.ResolveItemLink(item)
		}
		if guid == "" {
			guid = item.Title
		}
		if guid == "" {
			continue
		}

		if err := s.repo.MarkItemSeen(r.Context(), feed.ID, guid); err == nil {
			count++
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"message":      "Feed caught up successfully",
		"items_marked": count,
	})
}

// handleRewindFeed resets seen flags for the last N items.
func (s *Server) handleRewindFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	var payload rewindPayload
	payload.Limit = 10 // Default fallback limit
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}

	if payload.Limit <= 0 {
		s.writeError(w, http.StatusBadRequest, "Limit must be a positive integer")
		return
	}

	if err := s.repo.UnmarkSeenItems(r.Context(), id, payload.Limit); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"message": "Feed rewind executed successfully"})
}

// handleScanFeed triggers an immediate crawl/scan of the feed.
func (s *Server) handleScanFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	feed, err := s.repo.GetFeed(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Feed not found")
		return
	}

	triggered := s.scheduler.TriggerCrawl(context.Background(), feed)
	if !triggered {
		s.writeError(w, http.StatusConflict, "Feed scan is already in progress")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"message": "Feed scan triggered successfully"})
}

// generateMagicLinkToken yields an HMAC hex string verification key.
func generateMagicToken(email, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(email))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyMagicToken returns true if the incoming signature matches the expected HMAC.
func verifyMagicToken(email, token, secret string) bool {
	expected := generateMagicToken(email, secret)
	return hmac.Equal([]byte(token), []byte(expected))
}

// lineLevel classifies a raw log line by its severity level.
// Returns 0=DEBUG, 1=INFO, 2=WARN, 3=ERROR, or -1 if no level marker is found.
func lineLevel(line string) int {
	upper := strings.ToUpper(line)
	switch {
	case strings.Contains(upper, "LEVEL=ERROR") || strings.Contains(upper, " ERR ") || strings.HasSuffix(upper, " ERR"):
		return 3
	case strings.Contains(upper, "LEVEL=WARN") || strings.Contains(upper, " WRN ") || strings.HasSuffix(upper, " WRN"):
		return 2
	case strings.Contains(upper, "LEVEL=INFO") || strings.Contains(upper, " INF ") || strings.HasSuffix(upper, " INF"):
		return 1
	case strings.Contains(upper, "LEVEL=DEBUG") || strings.Contains(upper, " DBG ") || strings.HasSuffix(upper, " DBG"):
		return 0
	default:
		return -1
	}
}

type feedItemResponse struct {
	Title       string     `json:"title"`
	Link        string     `json:"link"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	GUID        string     `json:"guid"`
	Seen        bool       `json:"seen"`
}

// handleGetFeedItems parses the feed's remote URL in real-time and returns the recent 15 items with their "seen" audit status.
func (s *Server) handleGetFeedItems(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	feed, err := s.repo.GetFeed(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Feed not found")
		return
	}

	// Force fresh parse
	testFeed := *feed
	testFeed.ETag = ""
	testFeed.LastModified = ""

	res, err := s.crawler.Crawl(r.Context(), &testFeed)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch feed items: %v", err))
		return
	}

	if res.Feed == nil {
		s.writeJSON(w, http.StatusOK, []feedItemResponse{})
		return
	}

	// Retrieve first 15 items
	limit := min(len(res.Feed.Items), 15)

	resp := make([]feedItemResponse, 0, limit)
	for i := range limit {
		item := res.Feed.Items[i]
		seen, err := s.repo.IsItemSeen(r.Context(), feed.ID, item.GUID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		resp = append(resp, feedItemResponse{
			Title:       item.Title,
			Link:        crawler.ResolveItemLink(item),
			PublishedAt: item.PublishedParsed,
			GUID:        item.GUID,
			Seen:        seen,
		})
	}

	s.writeJSON(w, http.StatusOK, resp)
}
