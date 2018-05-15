/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/jasonlvhit/gocron"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/youtube/v3"
)

type chanChan chan chan struct{}

var (
	filename       = flag.String("filename", "", "Filename to upload. Can be a URL")
	thumbnail      = flag.String("thumbnail", "", "Thumbnail to upload. Can be a URL")
	title          = flag.String("title", "Video Title", "Video title")
	description    = flag.String("description", "uploaded by youtubeuploader", "Video description")
	categoryId     = flag.String("categoryId", "", "Video category Id")
	tags           = flag.String("tags", "", "Comma separated list of video tags")
	privacy        = flag.String("privacy", "private", "Video privacy status")
	quiet          = flag.Bool("quiet", false, "Suppress progress indicator")
	rate           = flag.Int("ratelimit", 0, "Rate limit upload in kbps. No limit by default")
	metaJSON       = flag.String("metaJSON", "", "JSON file containing title,description,tags etc (optional)")
	limitBetween   = flag.String("limitBetween", "", "Only rate limit between these times e.g. 10:00-14:00 (local time zone)")
	headlessAuth   = flag.Bool("headlessAuth", false, "set this if no browser available for the oauth authorisation step")
	showAppVersion = flag.Bool("v", false, "show version")
	chunksize      = flag.Int("chunksize", googleapi.DefaultUploadChunkSize, "size (in bytes) of each upload chunk. A zero value will cause all data to be uploaded in a single request")

	// this is set by compile-time to match git tag
	appVersion string = "unknown"

	// Globals
	db       *Store
	client   *http.Client
	settings *Settings
)

func init() {
	settings = loadSettings("conf.toml")

	db = openDatabase(settings.DbName)
	db.initializeDatabase()

	gocron.Every(settings.Interval).Minutes().Do(checkForNewUploads)
}

func main() {
	watcher, err := fsnotify.NewWatcher()
	handleErr(err)
	defer watcher.Close()

	done := make(chan bool)

	go func() {
		for {
			select {
			case ev := <-watcher.Events:
				if isCreateEvent(ev.Op) && isVideoFile(ev.Name) {
					v := Video{
						Filename: ev.Name,
					}
					fmt.Printf("Saving new video file '%s'...\n", ev.Name)
					db.saveVideo(v)
				}
			case err := <-watcher.Errors:
				handleErr(err)
			}
		}
	}()

	err = watcher.Add(settings.Folder)
	handleErr(err)

	<-gocron.Start()
	<-done
}

func isCreateEvent(op fsnotify.Op) bool {
	return op&fsnotify.Create == fsnotify.Create
}

func isVideoFile(filename string) bool {
	return strings.Contains(filename, settings.Extension)
}

func checkForNewUploads() {
	vids := db.getNotUploadedVideos()
	for _, v := range vids {
		fmt.Printf("Attemping to upload '%s'...\n", v.Filename)
		err := upload(v.Filename, "Uploaded from script")
		if err == nil {
			db.setVideoUploaded(*v)
		}
	}
}

func handleErr(err error) {
	if err != nil {
		panic(err)
	}
}

func upload(filename string, title string) error {
	var limitRange limitRange

	reader, filesize := Open(filename)
	defer reader.Close()

	ctx := context.Background()
	transport := &limitTransport{rt: http.DefaultTransport, lr: limitRange, filesize: filesize}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
		Transport: transport,
	})

	var quitChan chanChan
	if !*quiet {
		quitChan = make(chanChan)
		go func() {
			Progress(quitChan, transport, filesize)
		}()
	}
	client, err := buildOAuthHTTPClient(ctx, []string{youtube.YoutubeUploadScope, youtube.YoutubeScope})
	if err != nil {
		log.Fatalf("Error building OAuth client: %v", err)
	}

	upload := &youtube.Video{
		Snippet:          &youtube.VideoSnippet{},
		RecordingDetails: &youtube.VideoRecordingDetails{},
		Status:           &youtube.VideoStatus{},
	}

	//videoMeta := LoadVideoMeta(*metaJSON, upload)

	service, err := youtube.New(client)
	if err != nil {
		log.Fatalf("Error creating Youtube client: %s", err)
	}

	if upload.Status.PrivacyStatus == "" {
		upload.Status.PrivacyStatus = *privacy
	}
	if upload.Snippet.Tags == nil && strings.Trim(*tags, "") != "" {
		upload.Snippet.Tags = strings.Split(*tags, ",")
	}
	if upload.Snippet.Title == "" {
		upload.Snippet.Title = title
	}
	if upload.Snippet.Description == "" {
		upload.Snippet.Description = *description
	}
	if upload.Snippet.CategoryId == "" && *categoryId != "" {
		upload.Snippet.CategoryId = *categoryId
	}

	fmt.Printf("Uploading file '%s'...\n", filename)

	var option googleapi.MediaOption
	var video *youtube.Video

	option = googleapi.ChunkSize(*chunksize)

	call := service.Videos.Insert("snippet,status,recordingDetails", upload)
	video, err = call.Media(reader, option).Do()

	if quitChan != nil {
		quit := make(chan struct{})
		quitChan <- quit
		<-quit
	}

	if err != nil {
		if video != nil {
			log.Fatalf("Error making YouTube API call: %v, %v", err, video.HTTPStatusCode)
		} else {
			log.Fatalf("Error making YouTube API call: %v", err)
		}
		return err
	}
	fmt.Printf("Upload successful! Video ID: %v\n", video.Id)
	return nil
}
