package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/cavaliercoder/grab"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/sethvargo/go-retry"
)

const (
	apiKey            = ""
	apiKeySecret      = ""
	bearerToken       = ""
	accessToken       = ""
	accessTokenSecret = ""
)

func main() {
	err := do()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func do() error {
	config := oauth1.NewConfig(apiKey, apiKeySecret)
	token := oauth1.NewToken(accessToken, accessTokenSecret)
	httpClient := config.Client(oauth1.NoContext, token)

	client := twitter.NewClient(httpClient)

	includeEntities := true
	tweets, _, err := client.Favorites.List(&twitter.FavoriteListParams{
		ScreenName:      "shasderias",
		Count:           200,
		IncludeEntities: &includeEntities,
	})
	if err != nil {
		return err
	}

	fmt.Printf("got %d tweets\n", len(tweets))

	tweetsJSON, err := json.Marshal(tweets)
	if err != nil {
		return err
	}

	likesFileName := time.Now().UTC().Format("2006-01-02T15-04-05Z-likes.json")
	err = ioutil.WriteFile(likesFileName, tweetsJSON, 0644)
	if err != nil {
		return err
	}

	fmt.Println("tweets saved to likes.json")

	for i, tweet := range tweets {
		fmt.Printf("processing tweet #%d/%d - %s\n", i+1, len(tweets), tweet.IDStr)
		ct, err := tweet.CreatedAtTime()
		if err != nil {
			return err
		}

		dirName := fmt.Sprintf("%s-%s",
			ct.UTC().Format("2006-01-02T15-04-05Z"), tweet.IDStr)

		err = os.Mkdir(dirName, 0755)
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}

		tweetJSON, err := json.Marshal(tweet)
		if err != nil {
			return err
		}

		tweetJSONFilename := fmt.Sprintf("%s.json", tweet.IDStr)

		err = ioutil.WriteFile(filepath.Join(dirName, tweetJSONFilename), tweetJSON, 0644)
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}

		if tweet.ExtendedEntities == nil {
			fmt.Println("no media, continuing")
			goto Saved
		}
		for j, m := range tweet.ExtendedEntities.Media {
			fmt.Printf("downloading media #%d/%d\n", j+1, len(tweet.ExtendedEntities.Media))
			mediaURL := m.MediaURLHttps + "?name=large"
			downloadPath := filepath.Join(dirName, path.Base(m.MediaURLHttps))

			if _, err := os.Stat(downloadPath); err == nil {
				fmt.Printf("media downloaded, skipping\n")
				continue
			}

			b, err := retry.NewExponential(2 * time.Second)
			if err != nil {
				return err
			}

			b = retry.WithMaxRetries(4, b)

			err = retry.Do(context.Background(), b, func(ctx context.Context) error {
				_, err := grab.Get(downloadPath, mediaURL)
				if err != nil {
					fmt.Println("error downloading", mediaURL, ":", err)
					return retry.RetryableError(err)
				}
				return nil
			})

			if err != nil {
				fmt.Println("error downloading, retry limit reached, skipping", mediaURL, ":", err)
			}
		}

	Saved:
		fmt.Println("deleting")
		_, _, err = client.Favorites.Destroy(&twitter.FavoriteDestroyParams{
			ID: tweet.ID,
		})
		if err != nil {
			fmt.Println("error deleting", err)
		}
	}
	return nil
}
