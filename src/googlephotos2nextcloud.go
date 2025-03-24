package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PhotoMetadata represents the structure of the JSON metadata file accompanying each photo.
type PhotoMetadata struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	ImageViews     string   `json:"imageViews"`
	CreationTime   TimeData `json:"creationTime"`
	PhotoTakenTime TimeData `json:"photoTakenTime"`
	GeoData        GeoData  `json:"geoData"`
	People         []Person `json:"people"`
	URL            string   `json:"url"`
	Origin         Origin   `json:"googlePhotosOrigin"`
}

type TimeData struct {
	Timestamp string `json:"timestamp"`
	Formatted string `json:"formatted"`
}

type GeoData struct {
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Altitude      float64 `json:"altitude"`
	LatitudeSpan  float64 `json:"latitudeSpan"`
	LongitudeSpan float64 `json:"longitudeSpan"`
}

type Person struct {
	Name string `json:"name"`
}

type Origin struct {
	MobileUpload MobileUpload `json:"mobileUpload"`
}

type MobileUpload struct {
	DeviceFolder DeviceFolder `json:"deviceFolder"`
	DeviceType   string       `json:"deviceType"`
}

type DeviceFolder struct {
	LocalFolderName string `json:"localFolderName"`
}

type PhotoToUpload struct {
	FilePath  string
	SubFolder string
}

var photosList []PhotoToUpload

// uploadFile uploads a file to Nextcloud.
func uploadFile(filePath, nextcloudURL, username, password, subFolder string) error {
	fileName := filepath.Base(filePath)
	url := fmt.Sprintf("%s/%s/%s", nextcloudURL, subFolder, fileName)
	fmt.Printf("Uploading %s to %s\n", fileName, url)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	req, err := http.NewRequest("PUT", url, file)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload %s due to %s", fileName, resp.Status)
	} else {
		log.Printf("Successfully uploaded %s to %s\n", fileName, url)
	}

	return nil
}

// extractDateFolder extracts the year and month from a given timestamp (ISO 8601 or epoch).
func extractDateFolder(timestamp string) (string, error) {
	// Try to parse as ISO 8601 first
	parsedTime, err := time.Parse("2006-01-02T15:04:05Z", timestamp)
	if err == nil {
		return parsedTime.Format("2006/01"), nil
	}

	// If ISO 8601 fails, try to parse as epoch time
	epoch, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp format: %s", timestamp)
	}
	parsedTime = time.Unix(epoch, 0)
	return parsedTime.Format("2006/01"), nil
}

// getPhotoPathFromJSON extracts the corresponding photo path from the JSON file name.
func getPhotoPathFromJSON(jsonPath string) string {
	// Define possible JSON suffix patterns
	patterns := []string{".supplemental-metadata.json", ".suppl.json", ".suppl*.json", ".supplemental-metad.json"}
	return trimSuffixWildcard(jsonPath, patterns)
}

// trimSuffixWildcard trims a suffix based on a set of patterns.
func trimSuffixWildcard(s string, patterns []string) string {
	for _, pattern := range patterns {
		if strings.HasSuffix(s, pattern) {
			return strings.TrimSuffix(s, pattern)
		}
	}
	return s
}

// processDirectory recursively processes each photo and its metadata in the given directory.
func processDirectory(rootDir string) ([]PhotoToUpload, error) {
	result := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(info.Name(), ".json") {
			jsonPath := path
			photoPath := getPhotoPathFromJSON(jsonPath)

			if photoPath == "" {
				log.Printf("Photo file for %s does not exist, skipping.\n", jsonPath)
				return nil
			}

			// Read and parse the JSON metadata
			jsonFile, err := os.Open(jsonPath)
			if err != nil {
				log.Printf("Failed to open JSON file %s: %v\n", jsonPath, err)
				return nil
			}

			byteValue, err := ioutil.ReadAll(jsonFile)
			if err != nil {
				log.Printf("Failed to read JSON file %s: %v\n", jsonPath, err)
				jsonFile.Close()
				return nil
			}
			jsonFile.Close()

			var metadata PhotoMetadata
			json.Unmarshal(byteValue, &metadata)

			// Extract year/month folder
			subFolder, err := extractDateFolder(metadata.PhotoTakenTime.Timestamp)
			if err != nil {
				subFolder, err = extractDateFolder(metadata.CreationTime.Timestamp)
				if err != nil {
					log.Printf("Failed to extract date for %s: %v\n", photoPath, err)
					return nil
				}
			}

			// Add photo to list
			photosList = append(photosList, PhotoToUpload{
				FilePath:  photoPath,
				SubFolder: subFolder,
			})

		}
		return nil
	})
	return photosList, result
}

func main() {
	nextcloudURL := os.Getenv("NEXTCLOUD_URL")
	username := os.Getenv("NEXTCLOUD_USER")
	password := os.Getenv("NEXTCLOUD_PASSWORD")
	photosDir := os.Getenv("PHOTOS_DIR")

	if nextcloudURL == "" || username == "" || password == "" || photosDir == "" {
		log.Fatal("Missing required environment variables: NEXTCLOUD_URL, NEXTCLOUD_USER, NEXTCLOUD_PASSWORD, PHOTOS_DIR")
	}

	photosList, err := processDirectory(photosDir)
	if err != nil {
		log.Fatalf("Error processing directory: %v\n", err)
	} else {
		log.Printf("Successfully processed %d photos\n", len(photosList))

		// Create a channel to limit the number of concurrent uploads
		semaphore := make(chan struct{}, 10)

		// Create a wait group to wait for all uploads to finish
		var wg sync.WaitGroup

		// Upload photos in parallel
		for _, photo := range photosList {
			wg.Add(1)

			go func(photo PhotoToUpload) {
				defer wg.Done()

				// Acquire a semaphore to limit the number of concurrent uploads
				semaphore <- struct{}{}
				defer func() {
					<-semaphore
				}()

				// Upload the photo
				if err := uploadFile(photo.FilePath, nextcloudURL, username, password, photo.SubFolder); err != nil {
					log.Printf("Failed to upload photo %s: %v\n", photo.FilePath, err)
				}
			}(photo)
		}

		// Wait for all uploads to finish
		wg.Wait()
	}
}
