
# Google Photos to Nextcloud Uploader

This Dockerized Go application uploads photos and organizes them by year and month in Nextcloud.

## Usage

1. Ensure Docker and Docker Compose are installed.

2. Set the required environment variables:

    - `NEXTCLOUD_URL`: URL of your Nextcloud WebDAV endpoint (e.g., https://nextcloud.example.com/remote.php/dav/files/username)
    - `NEXTCLOUD_USER`: Nextcloud username
    - `NEXTCLOUD_PASSWORD`: Nextcloud password
    - `PHOTOS_DIR`: Abs path to google photos takeout dir (e.g., "/Users/edomsha/Desktop/photos/Takeout/Google Photos")

3. Place your Google Takeout photos (with JSON metadata) in the `photos` folder.

4. Build and run the uploader:

    ```bash
    docker-compose up --build
    ```

Photos will be uploaded and organized by year/month folders in Nextcloud.
