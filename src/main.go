package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/tajtiattila/metadata"
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

var (
	nextcloudURL, username, password, photosDir, parallel string
	myMap                                                 = make(map[string]string)
	failedCounter                                         = 0
	successfullCounter                                    = 0
)

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

func getMediaFileList(directory string) ([]string, []string) {
	var localJsonFileList []string
	var localMediaFileList []string

	// recursive search directory for files
	filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// check if file is folder and continue
		if info.IsDir() {
			return nil
		} else {
			if filepath.Ext(info.Name()) == ".json" {
				if strings.Count(info.Name(), ".") == 3 {
					localJsonFileList = append(localJsonFileList, path)
				}
			} else {
				localMediaFileList = append(localMediaFileList, path)
			}
		}
		return nil
	})

	return localJsonFileList, localMediaFileList
}

func parseExtractMetadatJsonFileAndAddToMapImage(jsonFileList []string) {
	// parse media metadata json file and get associated media file name and timestamp when it was created and add to map
	for _, jsonFile := range jsonFileList {
		parentPath := filepath.Dir(jsonFile)

		// Read and parse the JSON metadata
		openJsonFile, err := os.Open(jsonFile)
		if err != nil {
			log.Printf("Failed to open JSON file %s: %v\n", jsonFile, err)
			return
		}

		byteValue, err := ioutil.ReadAll(openJsonFile)
		if err != nil {
			log.Printf("Failed to read JSON file %s: %v\n", jsonFile, err)
			openJsonFile.Close()
			return
		}
		defer openJsonFile.Close()

		var metadata PhotoMetadata
		json.Unmarshal(byteValue, &metadata)

		fileName := metadata.Title
		photoTakenTime, _ := extractDateFolder(metadata.PhotoTakenTime.Timestamp)
		absImageFilePath := filepath.Join(parentPath, fileName)

		// Add photo to list
		myMap[absImageFilePath] = photoTakenTime
	}
}

func getMediaFilesWithoutMedtadataJsonFiles(mediaFileList []string) []string {
	var exifMEdiaFileList []string

	for _, mediaFile := range mediaFileList {
		_, exists := myMap[mediaFile]
		if !exists {
			exifMEdiaFileList = append(exifMEdiaFileList, mediaFile)
		} else {
			continue
		}
	}

	return exifMEdiaFileList
}

func parseExtractMediaFilesWithoutMedtadataJsonFileAddToMap(exifMEdiaFileList []string) {
	for _, photoPath := range exifMEdiaFileList {
		timeStamp := ""
		defaultTimestamp := "0001/01"
		if filepath.Ext(photoPath) == ".DS_Store" {
			continue
		}

		// Open the media file
		file, err := os.Open(photoPath)
		if err != nil {
			fmt.Println("Error opening file:", err)
			return
		}
		defer file.Close()

		// check if file is a directory
		if info, err := file.Stat(); err == nil && info.IsDir() {
			fmt.Println("Skipping directory:", info.Name())
			continue
		}

		// Parse metadata from the file
		meta, err := metadata.Parse(file)
		if err != nil {
			fmt.Printf("Error parsing file: [%s] with metadata: %v . Will use default value [%s]\n", photoPath, err, defaultTimestamp)
			timeStamp = defaultTimestamp
		}

		// Extract and print creation timestamp
		if timeStamp == "" {
			if meta.DateTimeCreated.IsZero() {
				fmt.Printf("Creation timestamp not found for file: [%s], going with original timestamp in EXIF data \n", photoPath)
				timeStamp = meta.DateTimeOriginal.Time.Format("2006/01")
			} else {
				timeStamp = meta.DateTimeCreated.Time.Format("2006/01")
			}

		}

		// Add photo to map
		_, exists := myMap[photoPath]
		if !exists {
			myMap[photoPath] = timeStamp
		} else {
			fmt.Println("Error, Media file already exists in map")
		}
	}
}

func GetEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func processDirectory(photosDir string) {
	// get media files from given directory
	jsonFileList, mediaFileList := getMediaFileList(photosDir)

	// parse media metadata json file and get associated media file name and timestamp when it was created and add to map
	parseExtractMetadatJsonFileAndAddToMapImage(jsonFileList)

	// get media files that do not exist in jsonFileList
	exifMEdiaFileList := getMediaFilesWithoutMedtadataJsonFiles(mediaFileList)

	// iterate over photoList and extract exif data and get metadata with timestamp
	parseExtractMediaFilesWithoutMedtadataJsonFileAddToMap(exifMEdiaFileList)

	for photoPath, subFolderTimestamp := range myMap {
		if strings.Contains(subFolderTimestamp, "0001/") {
			parts := strings.Split(subFolderTimestamp, "/")
			newSubFolderTimestamp := "2000/" + parts[1]
			myMap[photoPath] = newSubFolderTimestamp
		}
	}

	fmt.Printf("\n\nProcessed %d multimedia files \n\n", len(myMap))
}

// createNestedDirectories ensures all directories in the path exist on Nextcloud.
func createNestedDirectories(client *http.Client, baseURL, subFolder, username, password string) error {
	parts := strings.Split(subFolder, "/")
	currentPath := baseURL

	for _, part := range parts {
		if part == "" {
			continue
		}
		currentPath = fmt.Sprintf("%s/%s", currentPath, part)
		if err := createDirectoryIfNotExists(client, currentPath, username, password); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", currentPath, err)
		}
	}

	return nil
}

// createDirectoryIfNotExists checks if a WebDAV directory exists, and creates it if it doesn't.
func createDirectoryIfNotExists(client *http.Client, url, username, password string) error {
	req, err := http.NewRequest("MKCOL", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}

	if resp.StatusCode == 204 {
		log.Printf("Folder %s already exists in Nextcloud\n", url)
		return nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to create directory %s, status: %s", url, resp.Status)
	}

	log.Printf("Successfully created directory: %s\n", url)
	return nil
}

// uploadFile uploads a file to Nextcloud with retry on 404 status code.
func uploadFile(fileLocation, nextcloudURL, username, password, subFolder string) error {
	fileName := filepath.Base(fileLocation)
	url := fmt.Sprintf("%s/%s/%s", nextcloudURL, subFolder, fileName)
	absFileLocation, _ := filepath.Abs(fileLocation)

	retryCount := 3
	for attempt := 1; attempt <= retryCount; attempt++ {
		file, err := os.Open(absFileLocation)
		if err != nil {
			return err
		}
		defer file.Close()

		req, err := http.NewRequest("PUT", url, file)
		if err != nil {
			return err
		}
		req.SetBasicAuth(username, password)

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Disable certificate verification
		}
		client := &http.Client{Transport: transport}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
			successfullCounter++
			return nil
		}

		if resp.StatusCode == 204 {
			successfullCounter++
			return nil
		}

		// Retry on 404 status code
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGatewayTimeout {
			log.Printf("Attempt %d: Received %d for %s. Retrying...\n", attempt, resp.StatusCode, url)
			time.Sleep(2 * time.Second) // Wait before retrying
			continue
		}

		failedCounter++
		return fmt.Errorf("failed to upload %s due to %s", fileName, resp.Status)
	}

	failedCounter++
	return fmt.Errorf("failed to upload %s after %d retries", fileName, retryCount)
}

type MediaFile struct {
	Path string
	Ts   string
}

func uploadMediaFilesToNextcloud(parallelUploads int, nextcloudURL, username, password string, directories []string) {
	fmt.Println("Creating Required directories on Nextcloud")
	client := &http.Client{}
	dirSize := len(directories)

	numWorkers := runtime.NumCPU()
	fmt.Printf("Using %d workers (CPU cores)\n", numWorkers)

	// Initialize progress bar
	dirBar := progressbar.New(dirSize)

	// Create a channel to control the number of concurrent goroutines
	dirJobs := make(chan string, dirSize)
	// Create a wait group to wait for all goroutines to complete
	var wgDir sync.WaitGroup
	wgDir.Add(dirSize)

	// Create a fixed number of goroutines to handle the uploads
	for range parallelUploads {
		go func() {
			for directory := range dirJobs {
				// Ensure nested directories exist
				if err := createNestedDirectories(client, nextcloudURL, directory, username, password); err != nil {
					log.Printf("Error ensuring nested directories exist: %v \n", err)
				}
				dirBar.Add(1)
				wgDir.Done()
			}
		}()
	}

	// Iterate over the map and send each media file to the jobs channel
	for _, dir := range directories {
		dirJobs <- dir
	}

	// Close the jobs channel to signal that all jobs have been sent
	close(dirJobs)
	// Wait for all goroutines to complete
	wgDir.Wait()

	fmt.Println()

	fmt.Println("Uploading media files to Nextcloud")

	// Initialize progress bar
	mediaSize := len(myMap)

	mediaProgressBar := progressbar.New(mediaSize)

	jobs := make(chan MediaFile, mediaSize)
	progressChan := make(chan int, parallelUploads)
	var wgMedia sync.WaitGroup

	for range parallelUploads {
		wgMedia.Add(1)
		go worker(jobs, progressChan, &wgMedia)
	}

	// Send jobs (keys of the map) to workers
	go func() {
		for photoPath, subFolderTimestamp := range myMap {
			jobs <- MediaFile{photoPath, subFolderTimestamp}
		}
		close(jobs) // Close jobs channel after sending all keys
	}()

	// Close progress channel once all workers are done
	go func() {
		wgMedia.Wait()
		close(progressChan)
	}()

	finishCounter := 0
	// Update progress bar in real-time
	for p := range progressChan {
		finishCounter += p
		fmt.Printf("Uploaded %d/%d media files\n", finishCounter, mediaSize)
		_ = mediaProgressBar.Add(p)
	}
}

func worker(jobs chan MediaFile, progressChan chan int, wg *sync.WaitGroup) {
	defer wg.Done()

	for media := range jobs {
		// Upload the media file
		if err := uploadFile(media.Path, nextcloudURL, username, password, media.Ts); err != nil {
			log.Printf("Failed to upload file %s: [%v]\n", media.Path, err)
		}
		progressChan <- 1
	}
}

func getUniqueDirectoryToBecreatedOnNextCloud() []string {
	// Helper map to track unique values
	uniqueValuesMap := make(map[string]bool)

	// Slice to store unique values
	var uniqueValues []string

	// Iterate over the map and collect unique values
	for _, value := range myMap {
		if !uniqueValuesMap[value] {
			uniqueValuesMap[value] = true
			uniqueValues = append(uniqueValues, value)
		}
	}

	return uniqueValues
}

func main() {
	nextcloudURL = GetEnvWithDefault("NEXTCLOUD_URL", "")
	username = GetEnvWithDefault("NEXTCLOUD_USER", "")
	password = GetEnvWithDefault("NEXTCLOUD_PASSWORD", "")
	photosDir = GetEnvWithDefault("PHOTOS_DIR", "")
	parallel = GetEnvWithDefault("PARALLEL_UPLOADS", "1")

	if nextcloudURL == "" || username == "" || password == "" || photosDir == "" || parallel == "" {
		log.Fatal("Missing required environment variables: NEXTCLOUD_URL, NEXTCLOUD_USER, NEXTCLOUD_PASSWORD, PHOTOS_DIR, PARALLEL_UPLOADS")
	}

	// Convert string to integer
	parallelUploads, err := strconv.Atoi(parallel)
	if err != nil {
		fmt.Println("Error converting PARALLEL_UPLOADS string to integer:", err)
		return
	}

	// processDirectory(photosDir)
	processDirectory(photosDir)

	directoriesToBeCreated := getUniqueDirectoryToBecreatedOnNextCloud()

	uploadMediaFilesToNextcloud(parallelUploads, nextcloudURL, username, password, directoriesToBeCreated)

	fmt.Printf("\n\nSuccessfully uploaded %d media files \n\n", successfullCounter)
	fmt.Println("Failed to upload", failedCounter, "media files")
	os.Exit(0)
}
