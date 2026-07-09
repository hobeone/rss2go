//go:build uitest

package uitest

import (
	"context"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"rss2go/internal/types"
)

func TestFeedsTabSearching(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	// Seed feeds.
	feeds := []*types.Feed{
		{Title: "Tech News Feed", URL: "http://localhost/tech", PollIntervalSecs: 60, BackoffFactor: 1},
		{Title: "Science Daily", URL: "http://localhost/science", PollIntervalSecs: 60, BackoffFactor: 1},
		{Title: "Cooking Tips", URL: "http://localhost/cooking", PollIntervalSecs: 60, BackoffFactor: 1},
	}
	for _, f := range feeds {
		if err := env.Repo.CreateFeed(ctx, f); err != nil {
			t.Fatalf("CreateFeed: %v", err)
		}
	}

	page := env.newPage(t)
	screenshotOnFailure(t, page)

	env.navigate(t, page, "/")

	// Wait for feeds grid to load.
	if err := page.GetByText("Tech News Feed").WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Fatalf("feed card not visible: %v", err)
	}

	// Type "Tech" in filter box.
	filterInput := page.GetByPlaceholder("Search feed title or URL...")
	if err := filterInput.Fill("Tech"); err != nil {
		t.Fatalf("fill filter: %v", err)
	}

	// Tech News Feed should be visible.
	if err := page.GetByText("Tech News Feed").WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Errorf("Tech News Feed should be visible: %v", err)
	}

	// Science Daily should be hidden.
	if err := page.GetByText("Science Daily").WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateHidden,
	}); err != nil {
		t.Errorf("Science Daily should be hidden: %v", err)
	}
}

func TestFeedAddAndDelete(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	page := env.newPage(t)
	screenshotOnFailure(t, page)

	env.navigate(t, page, "/")

	// Click "+ Add Feed Source".
	addBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add Feed Source"})
	if err := addBtn.Click(); err != nil {
		t.Fatalf("click Add Feed Source button: %v", err)
	}

	// Fill Feed form.
	titleInput := page.GetByPlaceholder("Engineering Blog")
	if err := titleInput.Fill("E2E Test Feed"); err != nil {
		t.Fatalf("fill Title: %v", err)
	}
	urlInput := page.GetByPlaceholder("https://site.com/feed.xml")
	if err := urlInput.Fill(env.BaseURL + "/testfeed.xml"); err != nil {
		t.Fatalf("fill URL: %v", err)
	}

	// Submit form.
	saveBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Register Feed"})
	if err := saveBtn.Click(); err != nil {
		t.Fatalf("click save: %v", err)
	}

	// Wait for feed card to appear.
	card := page.Locator("h3").GetByText("E2E Test Feed")
	if err := card.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("new feed card not visible: %v", err)
	}

	// Click feed card to open popup.
	if err := card.Click(); err != nil {
		t.Fatalf("click feed card: %v", err)
	}

	// Click "Run Scan Now" button in the popup.
	scanBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Run Scan Now"})
	if err := scanBtn.Click(); err != nil {
		t.Fatalf("click Run Scan Now: %v", err)
	}

	// Wait for the scan toast notification.
	toast := page.GetByText("Feed scan triggered")
	if err := toast.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("scan toast not visible: %v", err)
	}

	// Click Delete button in the popup.
	deleteBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delete"})
	if err := deleteBtn.Click(); err != nil {
		t.Fatalf("click Delete: %v", err)
	}

	// Wait for feed card to disappear.
	if err := card.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateHidden,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Errorf("feed card not removed: %v", err)
	}
}

func TestFeedDetailsPopupAndItems(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	// Seed feed pointing to mock endpoint on local server!
	feed := &types.Feed{
		Title:              "Hermetic Feed",
		URL:                env.BaseURL + "/testfeed.xml",
		PollIntervalSecs:   60,
		BackoffFactor:      1,
		ExtractFullArticle: false,
	}
	if err := env.Repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	// Mark Mock Item 1 as seen in DB!
	if err := env.Repo.MarkItemSeen(ctx, feed.ID, "mock-guid-1"); err != nil {
		t.Fatalf("MarkItemSeen: %v", err)
	}

	page := env.newPage(t)
	screenshotOnFailure(t, page)

	env.navigate(t, page, "/")

	// Click feed card.
	card := page.Locator("h3").GetByText("Hermetic Feed")
	if err := card.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Fatalf("Hermetic Feed card not visible: %v", err)
	}
	if err := card.Click(); err != nil {
		t.Fatalf("click card: %v", err)
	}

	// Assert popup opens.
	header := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Hermetic Feed", Level: playwright.Int(2)})
	if err := header.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Fatalf("details header not visible: %v", err)
	}

	// Wait for feed items section to load.
	item1Link := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Mock Item 1"})
	if err := item1Link.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("Mock Item 1 link not visible: %v", err)
	}

	item2Link := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Mock Item 2"})
	if err := item2Link.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Fatalf("Mock Item 2 link not visible: %v", err)
	}

	// Verify badges are rendered for Seen (Emailed) vs Unseen (Unseen) items!
	emailedBadge := page.GetByText("Emailed")
	if err := emailedBadge.First().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Errorf("Emailed badge not visible: %v", err)
	}

	unseenBadge := page.GetByText("Unseen")
	if err := unseenBadge.First().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Errorf("Unseen badge not visible: %v", err)
	}
}

func TestSubscribersSplitPanelLayout(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	// Seed subscriber and feed.
	user := &types.User{Email: "alice@example.com"}
	if err := env.Repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	feed := &types.Feed{Title: "Alice Feed", URL: "http://localhost/alice", PollIntervalSecs: 60, BackoffFactor: 1}
	if err := env.Repo.CreateFeed(ctx, feed); err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}

	page := env.newPage(t)
	screenshotOnFailure(t, page)

	env.navigate(t, page, "/")

	// Click "Subscribers" sidebar nav button.
	subscribersNav := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Subscribers"})
	if err := subscribersNav.Click(); err != nil {
		t.Fatalf("click Subscribers nav button: %v", err)
	}

	// User row should be visible on the left pane.
	userRow := page.GetByText("alice@example.com")
	if err := userRow.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("user row not visible: %v", err)
	}

	// Click user row.
	if err := userRow.Click(); err != nil {
		t.Fatalf("click user row: %v", err)
	}

	// Right detail card should show the managing email text.
	rightText := page.Locator("strong").GetByText("alice@example.com")
	if err := rightText.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(3000),
	}); err != nil {
		t.Errorf("right panel user detail not visible: %v", err)
	}

	// Checklist item "Alice Feed" should be visible.
	feedLabel := page.GetByText("Alice Feed")
	if err := feedLabel.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Errorf("checklist item Alice Feed not visible: %v", err)
	}
}

func TestSubscriberAddAndDelete(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	page := env.newPage(t)
	screenshotOnFailure(t, page)

	env.navigate(t, page, "/")

	// Click "Subscribers" sidebar nav button.
	subscribersNav := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Subscribers"})
	if err := subscribersNav.Click(); err != nil {
		t.Fatalf("click Subscribers nav button: %v", err)
	}

	// Fill email input (directly visible at the top).
	emailInput := page.GetByPlaceholder("subscriber@example.com")
	if err := emailInput.Fill("new_e2e_user@example.com"); err != nil {
		t.Fatalf("fill email: %v", err)
	}

	// Click save button inside add form (the submit button).
	saveBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add Subscriber"})
	if err := saveBtn.Click(); err != nil {
		t.Fatalf("click submit Add Subscriber: %v", err)
	}

	// Wait for user row to appear in master list.
	userRow := page.GetByText("new_e2e_user@example.com")
	if err := userRow.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("new user row not visible: %v", err)
	}

	// Delete user by clicking the Remove button in the row.
	row := page.Locator("tr.subscriber-row", playwright.PageLocatorOptions{
		HasText: "new_e2e_user@example.com",
	})
	deleteBtn := row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Remove"})
	if err := deleteBtn.Click(); err != nil {
		t.Fatalf("click user Remove button: %v", err)
	}

	// Wait for user row to disappear.
	if err := userRow.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateHidden,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Errorf("user row not removed: %v", err)
	}
}

func TestFeedEditFlow(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	page := env.newPage(t)
	screenshotOnFailure(t, page)

	env.navigate(t, page, "/")

	// Click "+ Add Feed Source".
	addBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Add Feed Source"})
	if err := addBtn.Click(); err != nil {
		t.Fatalf("click Add Feed Source button: %v", err)
	}

	// Fill Feed form.
	titleInput := page.GetByPlaceholder("Engineering Blog")
	if err := titleInput.Fill("E2E Edit Test Feed"); err != nil {
		t.Fatalf("fill Title: %v", err)
	}
	urlInput := page.GetByPlaceholder("https://site.com/feed.xml")
	if err := urlInput.Fill(env.BaseURL + "/testfeed.xml"); err != nil {
		t.Fatalf("fill URL: %v", err)
	}

	// Submit form.
	saveBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Register Feed"})
	if err := saveBtn.Click(); err != nil {
		t.Fatalf("click save: %v", err)
	}

	// Wait for feed card to appear.
	card := page.Locator("h3").GetByText("E2E Edit Test Feed")
	if err := card.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("new feed card not visible: %v", err)
	}

	// Click feed card to open details popup.
	if err := card.Click(); err != nil {
		t.Fatalf("click feed card: %v", err)
	}

	// Click "Edit Feed Config" button in the popup.
	editBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Edit Feed Config"})
	if err := editBtn.Click(); err != nil {
		t.Fatalf("click Edit Feed Config: %v", err)
	}

	// Wait for Edit Form overlay to be visible.
	editTitleInput := page.Locator("input[placeholder='Engineering Blog']")
	if err := editTitleInput.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		t.Fatalf("edit title input not visible: %v", err)
	}

	// Change Title.
	if err := editTitleInput.Fill("E2E Edited Title"); err != nil {
		t.Fatalf("fill edited Title: %v", err)
	}

	// Click "Save Config Changes".
	saveChangesBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save Config Changes"})
	if err := saveChangesBtn.Click(); err != nil {
		t.Fatalf("click Save Config Changes: %v", err)
	}

	// The detail popup should still be open and display the new title.
	popupTitle := page.Locator("h2").GetByText("E2E Edited Title")
	if err := popupTitle.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("expected feed detail popup to be open with edited title: %v", err)
	}

	// Click '✕' to close the detail popup.
	closeBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "✕"})
	if err := closeBtn.Click(); err != nil {
		t.Fatalf("click close popup: %v", err)
	}
}
