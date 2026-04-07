package main

import (
	"context"
	"fmt"

	"github.com/hobeone/rss2go/internal/db"
	"github.com/spf13/cobra"
)

var (
	userCmd = &cobra.Command{
		Use:   "user",
		Short: "Manage users and subscriptions",
	}

	userAddCmd = &cobra.Command{
		Use:   "add [email]",
		Short: "Add a new user",
		Args:  cobra.ExactArgs(1),
		RunE:  runAddUser,
	}

	userSubscribeCmd = &cobra.Command{
		Use:   "subscribe [email] [feed-id or url]",
		Short: "Subscribe a user to a feed",
		Args:  cobra.ExactArgs(2),
		RunE:  runSubscribe,
	}

	userUnsubscribeCmd = &cobra.Command{
		Use:   "unsubscribe [email] [feed-id or url]",
		Short: "Unsubscribe a user from a feed",
		Args:  cobra.ExactArgs(2),
		RunE:  runUnsubscribe,
	}
)

// getFeedID resolves a feed ID from either a numeric string or a URL.
func getFeedID(ctx context.Context, store db.Store, arg string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(arg, "%d", &id); err == nil {
		return id, nil
	}

	f, err := store.GetFeedByURL(ctx, arg)
	if err != nil {
		return 0, err
	}
	if f == nil {
		return 0, fmt.Errorf("feed not found with URL: %s", arg)
	}
	return f.ID, nil
}

func init() {
	userCmd.AddCommand(userAddCmd)
	userCmd.AddCommand(userSubscribeCmd)
	userCmd.AddCommand(userUnsubscribeCmd)
	rootCmd.AddCommand(userCmd)
}

func runAddUser(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	id, err := store.AddUser(context.Background(), args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Added user: %s (ID: %d)\n", args[0], id)
	return nil
}

func runSubscribe(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	user, err := store.GetUserByEmail(ctx, args[0])
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found: %s", args[0])
	}

	feedID, err := getFeedID(ctx, store, args[1])
	if err != nil {
		return err
	}

	if err := store.Subscribe(ctx, user.ID, feedID); err != nil {
		return err
	}
	fmt.Printf("Subscribed %s to feed ID %d\n", args[0], feedID)
	return nil
}

func runUnsubscribe(cmd *cobra.Command, args []string) error {
	_, _, store, err := setup()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	user, err := store.GetUserByEmail(ctx, args[0])
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found: %s", args[0])
	}

	feedID, err := getFeedID(ctx, store, args[1])
	if err != nil {
		return err
	}

	if err := store.Unsubscribe(ctx, user.ID, feedID); err != nil {
		return err
	}
	fmt.Printf("Unsubscribed %s from feed ID %d\n", args[0], feedID)
	return nil
}
