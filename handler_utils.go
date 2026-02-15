package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

const (
	GiB = 1 << 30
	MiB = 1 << 20
)

func validateRequest(cfg *apiConfig, w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, error) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return uuid.UUID{}, uuid.UUID{}, err
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return uuid.UUID{}, uuid.UUID{}, err
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return uuid.UUID{}, uuid.UUID{}, err
	}

	return videoID, userID, nil
}

func parseUploadReq(w http.ResponseWriter, r *http.Request, key string) (string, multipart.File, error) {
	validMediaTypes := make(map[string]struct{})
	switch key {
	case "thumbnail":
		validMediaTypes["image/png"] = struct{}{}
		validMediaTypes["image/jpeg"] = struct{}{}
	case "video":
		validMediaTypes["video/mp4"] = struct{}{}
	default:
		panic("Invalid key for Content-Type header")
	}

	const maxMemory = 10 * MiB
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form", err)
		return "", nil, err
	}

	multipartFile, header, err := r.FormFile(key)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't form file the request", err)
		return "", nil, err
	}

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if _, ok := validMediaTypes[mediaType]; !ok {
		err = errors.New("Invalid media type")
		respondWithError(w, http.StatusBadRequest, "Thumbnail media type must be either image/jpeg or image/png", err)
		return "", nil, err
	}

	return mediaType, multipartFile, nil
}

func getVideoMetadata(cfg *apiConfig, w http.ResponseWriter, videoID, userID uuid.UUID) (database.Video, error) {
	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find image metdata in db", err)
	} else if metadata.UserID != userID {
		err = errors.New("User ID from request does not match video owner ID")
		http.Error(w, err.Error(), http.StatusUnauthorized)
	}

	return metadata, err
}

func (cfg *apiConfig) updateThumbnail(multipartFile multipart.File, mediaType string, metadata database.Video) error {
	randBytes := make([]byte, 32)
	if _, err := rand.Read(randBytes); err != nil {
		return err
	}
	randEncoded := base64.RawURLEncoding.EncodeToString(randBytes)

	fileExt := strings.Split(mediaType, "/")[1]
	fileName := randEncoded + "." + fileExt
	thumbnailFilePath := filepath.Join(cfg.assetsRoot, fileName)

	file, err := os.Create(thumbnailFilePath)
	if err != nil {
		return err
	}

	if _, err = io.Copy(file, multipartFile); err != nil {
		return err
	}

	tnURL := "http://localhost:" + cfg.port + "/assets/" + fileName
	metadata.ThumbnailURL = &tnURL
	cfg.db.UpdateVideo(metadata)

	log.Println("Info: thumbnail URL updated in db")
	return nil
}

func (cfg *apiConfig) updateVideo(tempFile *os.File, orientation, mediaType string, metadata database.Video) error {
	randBytes := make([]byte, 32)
	if _, err := rand.Read(randBytes); err != nil {
		return err
	}
	s3Key := orientation + "/" + base64.RawURLEncoding.EncodeToString(randBytes) + ".ext"

	if _, err := cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &s3Key,
		Body:        tempFile,
		ContentType: &mediaType,
	}); err != nil {
		return err
	}

	videoURL := "https://" + cfg.s3Bucket + ".s3." + cfg.s3Region + ".amazonaws.com/" + s3Key
	metadata.VideoURL = &videoURL
	cfg.db.UpdateVideo(metadata)

	log.Println("Info: video", videoURL, "uploaded to s3Bucket and metadata stored in db")

	return nil
}

type ffprobeOutput struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var output bytes.Buffer
	cmd.Stdout = &output

	if err := cmd.Run(); err != nil {
		return "", err
	}

	var videoDimensions ffprobeOutput
	if err := json.Unmarshal(output.Bytes(), &videoDimensions); err != nil {
		return "", err
	}

	width := videoDimensions.Streams[0].Width
	height := videoDimensions.Streams[0].Height
	ratio := float64(width) / float64(height)
	log.Println("Aspect ratio found", ratio)

	const tolerance = 0.02 // 2% tolerance

	switch {
	case math.Abs(ratio-16.0/9.0) < tolerance: // ~1.778
		return "16:9", nil
	case math.Abs(ratio-9.0/16.0) < tolerance: // ~0.5625
		return "9:16", nil
	default:
		return "other", nil
	}
}
