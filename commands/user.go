package commands

import (
	"fmt"

	"github.com/hobeone/rss2go/config"
	"github.com/hobeone/rss2go/db"
	"gopkg.in/alecthomas/kingpin.v2"
)

type userCommand struct {
	Config *config.Config
	DBH    *db.Handle
	Name   string
	Email  string
	Pass   string
	Feeds  []string
}

func (uc *userCommand) init() {
	uc.Config, uc.DBH = commonInit()
}

func (uc *userCommand) configure(app *kingpin.Application) {
	userCmd := app.Command("users", "manipulate users")

	userCmd.Command("list", "Show all users").Action(uc.listCmd)

	addCmd := userCmd.Command("add", "Add a new user").Action(uc.addCmd)
	addCmd.Arg("username", "Name of user").Required().StringVar(&uc.Name)
	addCmd.Arg("email", "Email address of user").Required().StringVar(&uc.Email)
	addCmd.Arg("password", "Password of user").Required().StringVar(&uc.Pass)
	addCmd.Arg("feeds", "URLs of feeds to subscribe to (optional)").StringsVar(&uc.Feeds)
}

func (uc *userCommand) listCmd(c *kingpin.ParseContext) error {
	uc.init()
	return uc.list()
}
func (uc *userCommand) list() error {
	users, err := uc.DBH.GetAllUsers()
	if err != nil {
		return err
	}
	if len(users) > 0 {
		for _, u := range users {
			fmt.Printf("%d: %#v, %#v\n", u.ID, u.Name, u.Email)
			feeds, err := uc.DBH.GetUsersFeeds(&u)
			if err != nil {
				return fmt.Errorf("Couldn't find users subscriptions: %s", err)
			}
			for _, fi := range feeds {
				fmt.Printf("  %s - %s\n", fi.Name, fi.URL)
			}
			fmt.Println("")
		}
	} else {
		fmt.Println("No Users found.")
	}
	return nil
}

func (uc *userCommand) addCmd(c *kingpin.ParseContext) error {
	uc.init()
	return uc.add()
}

func (uc *userCommand) add() error {
	dbuser, err := uc.DBH.AddUser(uc.Name, uc.Email, uc.Pass)
	if err != nil {
		return err
	}
	fmt.Printf("Added user %s <%s>.\n", uc.Name, uc.Email)

	if len(uc.Feeds) > 0 {
		feeds := make([]*db.FeedInfo, len(uc.Feeds))
		for i, feedURL := range uc.Feeds {
			f, err := uc.DBH.GetFeedByURL(feedURL)
			if err != nil {
				return fmt.Errorf("Feed %s doesn't exist in db, add it first.", feedURL)
			}
			feeds[i] = f
		}
		err = uc.DBH.AddFeedsToUser(dbuser, feeds)
		if err != nil {
			return fmt.Errorf("Error adding feeds to user: %s", err)
		}
	}
	return nil
}

func (uc *userCommand) deleteCmd(c *kingpin.ParseContext) error {
	uc.init()
	return uc.delete()
}

func (uc *userCommand) delete() error {
	err := uc.DBH.RemoveUserByEmail(uc.Email)
	if err != nil {
		return fmt.Errorf("Error removing user: %s", err)
	}
	fmt.Printf("Removed user %s.\n", uc.Email)
	return nil
}

func (uc *userCommand) subscribeCmd(c *kingpin.ParseContext) error {
	uc.init()
	return uc.subscribe()
}

func (uc *userCommand) subscribe() error {
	u, err := uc.DBH.GetUserByEmail(uc.Email)
	if err != nil {
		return fmt.Errorf("Error getting user '%s': %s", uc.Email, err)
	}
	feeds := make([]*db.FeedInfo, len(uc.Feeds))
	for i, feedURL := range uc.Feeds {
		f, err := uc.DBH.GetFeedByURL(feedURL)
		if err != nil {
			return fmt.Errorf("Feed %s doesn't exist in db, add it first.", feedURL)
		}
		feeds[i] = f
	}
	err = uc.DBH.AddFeedsToUser(u, feeds)
	if err != nil {
		return fmt.Errorf("Error adding feeds to user: %s", err)
	}

	fmt.Printf("Subscribed user %s to %d feed.\n", uc.Email, len(uc.Feeds))
	return nil
}

func (uc *userCommand) unsubscribeCmd(c *kingpin.ParseContext) error {
	uc.init()
	return uc.unsubscribe()
}

func (uc *userCommand) unsubscribe() error {
	u, err := uc.DBH.GetUserByEmail(uc.Email)
	if err != nil {
		return fmt.Errorf("Error getting user: %s", err)
	}

	feeds := []*db.FeedInfo{}
	for _, feedURL := range uc.Feeds {
		f, err := uc.DBH.GetFeedByURL(feedURL)
		if err != nil {
			fmt.Printf("Feed %s doesn't exist in db, skipping.\n", feedURL)
			continue
		}
		feeds = append(feeds, f)
	}
	if len(feeds) > 0 {
		err = uc.DBH.RemoveFeedsFromUser(u, feeds)
		if err != nil {
			return fmt.Errorf("Error removing feeds from user: %s", err)
		}
	}

	return nil
}