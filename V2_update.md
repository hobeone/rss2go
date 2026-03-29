# rss2go v2: Suggestions & Improvements

This document records the suggested features and improvements for a future v2 version of `rss2go`.

## 1. User Experience & Management

### 1.1 Web-Based Management UI
- **Current State**: All management (adding feeds, users, subscribing) is done via the CLI.
- **v2 Suggestion**: A built-in web dashboard for managing the system. This would include:
    - User authentication and role-based access (Admin vs. Regular User).
    - Graphical feed management (add, delete, pause/resume polling).
    - Subscription management for users.
    - View recent errors and crawler/mailer metrics.

### 1.2 Self-Service Subscriptions
- **v2 Suggestion**: Allow users to self-register and manage their own subscriptions.
    - Email-based sign-up/login.
    - A "My Subscriptions" page to easily add/remove feeds.
    - Integration of "Unsubscribe" links in every email sent.

### 1.3 OPML Import/Export
- **v2 Suggestion**: Support the industry-standard OPML format for bulk importing and exporting feed lists.

## 2. Notification & Content Features

### 2.1 Multi-Channel Notifications
- **Current State**: Only supports Email.
- **v2 Suggestion**: Add support for other notification platforms via a modular "Notifiers" system:
    - **Messaging Apps**: Telegram, Discord, Slack.
    - **Webhooks**: Generic POST requests for integration with other tools (e.g., Zapier, IFTTT).
    - **Push Notifications**: Support for browser push or mobile app push.

### 2.2 Advanced Content Filtering
- **v2 Suggestion**: Allow users to define filters for their subscriptions:
    - **Keyword Filtering**: Only receive items containing specific words (or excluding certain words).
    - **Regex Support**: Advanced pattern matching for titles or bodies.
    - **Tag Filtering**: For feeds that provide categories or tags.

### 2.3 Full-Article Extraction
- **v2 Suggestion**: Many RSS feeds only provide short summaries. v2 could include an optional "Full-Text" feature that fetches the source URL and extracts the main article content (similar to Readability or Mercury Parser).

### 2.4 Customizable Formatting & Templates
- **Current State**: Fixed formatting in `watcher.go`.
- **v2 Suggestion**: Use Go's `html/template` or a specialized engine to allow users (or admins) to customize the look of the notifications. Provide different templates for different notification channels.

### 2.5 Digest Emails
- **v2 Suggestion**: Instead of immediate delivery, offer "Digest" modes:
    - **Daily/Weekly Digests**: Combine all new items from a feed (or all subscriptions) into a single summary email.
    - **Configurable Schedule**: Let users choose when they want to receive their digests.

## 3. Performance & Architecture

### 3.1 Distributed/Microservices Architecture
- **Current State**: Single binary with internal worker pools.
- **v2 Suggestion**: Decouple the system into independent services:
    - **Crawler Service**: Focused on high-throughput HTTP fetching.
    - **Mailer/Notifier Service**: Handles the persistence and retries of outgoing messages.
    - **Core/Orchestrator**: Manages the state and logic.
    - This allows for independent scaling (e.g., running many crawlers in different geographic regions).

### 3.2 Enhanced Crawler Efficiency
- **v2 Suggestion**: Support for `ETag` and `If-Modified-Since` headers to avoid downloading the same feed content if it hasn't changed since the last poll, significantly saving bandwidth.

### 3.3 Database Pruning & Migration to PostgreSQL
- **v2 Suggestion**:
    - **Pruning**: Automatically delete "seen" items older than a certain age (e.g., 90 days) to prevent the SQLite database from growing indefinitely.
    - **PostgreSQL Support**: Add support for a more robust database like PostgreSQL for large-scale deployments.

### 3.4 Improved Observability
- **v2 Suggestion**: 
    - **Distributed Tracing**: Integration with OpenTelemetry to trace a feed item from its initial crawl through sanitization and out to the mailer.
    - **Health Dashboard**: A more detailed view of per-feed health, average polling times, and failure rates.

## 4. Security & Robustness

### 4.1 Scriptable Sanitization
- **v2 Suggestion**: Instead of fixed rules in `cleanFeedContent`, allow per-feed or global "Cleanup Scripts" (perhaps using a safe scripting language like Lua or Starlark) to handle complex site-specific DOM cleaning.

### 4.2 Graceful Shutdown & Queue Draining
- **v2 Suggestion**: Ensure that when the daemon stops, it waits for all currently processing items to be either successfully mailed or safely persisted for the next run.
