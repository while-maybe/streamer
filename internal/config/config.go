package config

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"streamer/internal/media"
	"strings"
	"time"
	"unicode"

	"github.com/gofrs/uuid/v5"
)

type HttpTimeoutsConfig struct {
	Read     time.Duration
	Idle     time.Duration
	Write    time.Duration
	Shutdown time.Duration // how long we give the shutdown process to gracefully terminate
}

type HTTPConfig struct {
	Addr     string
	Timeouts HttpTimeoutsConfig
}

type ShutdownTimersConfig struct {
	InactiveLimit time.Duration
	SleepTimer    time.Duration
	TimeToEnd     time.Time
}

type MediaConfig struct {
	Mode         media.ResourceMode // "direct" or "buffered"
	BufferSize   int
	FriendlyName string
	UUID         string
	Volumes      []VolumeConfig
}

type VolumeConfig struct {
	ID    string
	MaxIO int
	Paths []string
}

type LogConfig struct {
	Level slog.Level
}

type Config struct {
	HTTP           HTTPConfig
	ShutdownTimers ShutdownTimersConfig
	Media          MediaConfig
	Logger         LogConfig
}

type mountFlag []VolumeConfig

func (m *mountFlag) String() string {
	return "Mount definition: ID:Limit:Path1,Path2,..."
}

func (m *mountFlag) Set(value string) error {
	// Expected: "disk1:10:/mnt/a,/mnt/b,..."

	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid format, expected 'id:limit:path,path'")
	}

	id := parts[0]
	limit, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid limit: %w", err)
	}

	rawPaths := strings.SplitSeq(parts[2], ",")
	cleanPaths := []string{}
	for p := range rawPaths {
		if trimmedPath := strings.TrimSpace(p); trimmedPath != "" {
			cleanPaths = append(cleanPaths, trimmedPath)
		}
	}

	*m = append(*m, VolumeConfig{
		ID:    id,
		MaxIO: limit,
		Paths: cleanPaths,
	})

	return nil
}

const (
	defaultBufferSize = 10 * 1024 * 1024
	noTimeout         = time.Duration(0)
)

func DefaultConfig() *Config {
	return &Config{
		HTTP: HTTPConfig{
			Addr: ":8081",
			Timeouts: HttpTimeoutsConfig{
				Read:     5 * time.Second,
				Idle:     30 * time.Second,
				Write:    1 * time.Hour,
				Shutdown: 15 * time.Second,
			},
		},
		Media: MediaConfig{
			Mode:         media.ModeFileBuffered,
			BufferSize:   defaultBufferSize,
			FriendlyName: "GoStream Server",
			UUID:         "",
			Volumes:      []VolumeConfig{},
		},
		ShutdownTimers: ShutdownTimersConfig{
			InactiveLimit: 30 * time.Minute,
			SleepTimer:    noTimeout,
			TimeToEnd:     time.Time{},
		},
		Logger: LogConfig{
			Level: slog.LevelInfo,
		},
	}
}

func ParseArgs(cfg *Config, args []string, stderr io.Writer) error {
	defaultCfg := DefaultConfig()

	fs := flag.NewFlagSet("gomediaserver", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s [options] [path]\n\n", fs.Name())
		fmt.Fprintln(fs.Output(), "A DLNA/UPnP media server for streaming videos to Smart TVs and other devices.")
		fmt.Fprintln(fs.Output(), "\nOptions:")
		fs.PrintDefaults()
		fmt.Fprintln(fs.Output(), "\nArguments:")
		fmt.Fprintln(fs.Output(), "  path    Media root directory (default: current directory)")
	}

	fs.StringVar(&cfg.HTTP.Addr, "http.addr", defaultCfg.HTTP.Addr, "http address to listen on")

	var modeStr string
	fs.StringVar(&modeStr, "media.mode", "buffered", "Resource mode: direct, buffered")

	var bufferSizeStr string
	fs.StringVar(&bufferSizeStr, "media.bufferSize", "10MB", "Read buffer size (e.g. 10MB, 512KB)")

	var logLevelStr string
	fs.StringVar(&logLevelStr, "logger.level", "info", "Log level (debug, info, warn, error)")

	var friendlyNameStr string
	fs.StringVar(&friendlyNameStr, "media.friendlyName", defaultCfg.Media.FriendlyName, "DLNA server name (max 64 chars)")

	// we can store the parsing result in the cfg object as the default uuid is a blank string
	fs.StringVar(&cfg.Media.UUID, "media.uuid", defaultCfg.Media.UUID, "Server UUID (unique identifier). Generated randomly on startup if empty.")

	fs.DurationVar(&cfg.ShutdownTimers.InactiveLimit, "shutdown.inactive", defaultCfg.ShutdownTimers.InactiveLimit, "Shutdown after duration of inactivity (e.g. 30m)")

	fs.DurationVar(&cfg.ShutdownTimers.SleepTimer, "shutdown.sleep", defaultCfg.ShutdownTimers.SleepTimer, "Shutdown after specific duration (e.g. 2h)")

	var timeToEndStr string
	fs.StringVar(&timeToEndStr, "shutdown.at", "", "Shutdown at specific time (format HH:MM, e.g. 23:30)")

	var maxIO int
	// TODO make this a little better - magic number here?
	fs.IntVar(&maxIO, "media.maxIO", 10, "Max concurrent disk reads")

	var mounts mountFlag
	fs.Var(&mounts, "media.mount", "Mount grouped volumes: ID:Limit:Path1,Path2,...")

	// parse all flags
	if err := fs.Parse(args); err != nil {
		return err
	}

	// validate mode
	mode, err := validateMode(modeStr)
	if err != nil {
		return err
	}
	cfg.Media.Mode = mode

	// validate buffer.size
	bufferSize, err := validateBufferSize(bufferSizeStr)
	if err != nil {
		return err
	}
	cfg.Media.BufferSize = int(bufferSize)

	// validate logger.level
	level, err := validateLoggerLevel(logLevelStr)
	if err != nil {
		return err
	}
	cfg.Logger.Level = level

	// validate media.friendlyName
	friendlyName, err := validateFriendlyName(friendlyNameStr)
	if err != nil {
		return err
	}
	cfg.Media.FriendlyName = friendlyName

	// validate media.uuid
	mediaUuid, err := validateUUID(cfg.Media.UUID)
	if err != nil {
		return err
	}
	cfg.Media.UUID = mediaUuid

	// validate timeToEnd
	timeToEnd, err := validateTimeToEnd(timeToEndStr)
	if err != nil {
		return err
	}
	cfg.ShutdownTimers.TimeToEnd = timeToEnd

	// parse the mounts
	if len(mounts) > 0 {
		cfg.Media.Volumes = mounts
	}

	paths := fs.Args()

	// no mounts, no positional args, path becomes current folder
	if len(paths) == 0 && len(mounts) == 0 {
		paths = []string{"."}
	}

	// if there are any positional paths create a vol with them and append
	if len(paths) > 0 {
		positionalVol, err := NewVolumeConfig("", paths, maxIO)
		if err != nil {
			return err
		}
		cfg.Media.Volumes = append(cfg.Media.Volumes, positionalVol)
	}

	if err := cfg.validatePaths(); err != nil {
		return err
	}

	return nil
}

func NewVolumeConfig(id string, paths []string, maxIO int) (VolumeConfig, error) {
	// Strict Validation: no empty paths list
	if len(paths) == 0 {
		return VolumeConfig{}, fmt.Errorf("volume must have at least one path")
	}

	maxIO = max(1, maxIO)

	// assign new uuid if no id was provided
	if id == "" {
		uuid, err := uuid.NewV7()
		if err != nil {
			return VolumeConfig{}, fmt.Errorf("generate volume uuid: %w", err)
		}
		id = uuid.String()
	}

	return VolumeConfig{
		ID:    id,
		Paths: paths,
		MaxIO: maxIO,
	}, nil
}

func (cfg *Config) validatePaths() error {
	pathsInConfig := make(map[string]string)

	for _, vol := range cfg.Media.Volumes {
		for _, path := range vol.Paths {
			absPath, err := filepath.Abs(path)

			if err != nil {
				return fmt.Errorf("resolve path %q: %w", path, err)
			}

			if existingVol, ok := pathsInConfig[path]; ok {
				return fmt.Errorf("path %s is mounted on different vols (%s and %s)", path, existingVol, vol.ID)
			}

			pathsInConfig[absPath] = vol.ID
		}
	}
	return nil
}

func validateMode(modeStr string) (media.ResourceMode, error) {
	switch strings.ToLower(modeStr) {
	case "direct":
		return media.ModeFileDirect, nil
	case "buffered":
		return media.ModeFileBuffered, nil
	default:
		return media.ModeUnknown, fmt.Errorf("invalid mode %q: must be 'direct' or 'buffered'", modeStr)
	}
}

func validateBufferSize(bufStr string) (int, error) {
	bufSize64, err := parseBytes(bufStr)
	if err != nil {
		return 0, err
	}

	// check if it fits in architecture (as in potentially 32 bits)
	// do a xor on 0 to flip all bits to 1 and shift one bit to the right (leaving msb at zero)
	const maxInt = int(^uint(0) >> 1)
	if bufSize64 > int64(maxInt) {
		return 0, fmt.Errorf("buffer size too large for this system architecture")
	}

	// negative buffer size
	if bufSize64 < 0 {
		return 0, fmt.Errorf("buffer size cannot be negative")
	}
	return int(bufSize64), nil
}

func validateFriendlyName(fNameStr string) (string, error) {
	fNameStr = strings.TrimSpace(fNameStr)

	if fNameStr == "" {
		return "", fmt.Errorf("server name cannot be empty")
	}
	if len(fNameStr) > 64 {
		return "", fmt.Errorf("server name too long (max 64 chars, got %d)", len(fNameStr))
	}
	return fNameStr, nil
}

// done opts:
// --http.addr		string
// --mode			string	(default "buffered", options: "direct", "buffered")
// --buffer.size 	string	(default "10MB")
// --log-level		string	(default "info")
// --friendlyName	string	(default "GoStream Server")
// --media.uuid string		(optional, generate if not provided)
// --media.
// --shutdown.inactiveLimit	time.Duration	(default: 30mins)
// --shutdown.sleepTimer	time.Duration	(default: 0)
// --shutdown.timeToEnd		time.Time		(default: 0)

// done args:
// collect paths from positional arguments done!

// need doing:
// --root-path string			(default "/home/username/Videos")
// --port int					(default 8081)
// --shutdown-delay duration	(default 15s)

func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	// find the index of first rune representing size suffix
	i := strings.IndexFunc(s, func(r rune) bool {
		return !unicode.IsDigit(r) && r != '.'
	})

	// there is no unit
	if i == -1 {
		return strconv.ParseInt(s, 10, 64)
	}

	// numeric string in one var, unitStr in another
	numericStr := s[:i]
	unitStr := strings.TrimSpace(s[i:])

	val, err := strconv.ParseFloat(numericStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number in byte string: %w", err)
	}

	var multiplier float64
	switch unitStr {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown unit %q (expected B, KB, MB, GB)", unitStr)
	}

	return int64(val * multiplier), nil
}

func validateLoggerLevel(logLevelStr string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(logLevelStr)); err != nil {
		return level, fmt.Errorf("invalid log level %q: %w", logLevelStr, err)
	}
	return level, nil
}

func validateUUID(uuidStr string) (string, error) {
	// user did provide a uuid
	if uuidStr != "" {
		// check if user provided "uuid:" prefix
		cleanUuid := strings.TrimPrefix(uuidStr, "uuid:")
		id, err := uuid.FromString(cleanUuid)
		if err != nil {
			return "", fmt.Errorf("failed to parse UUID %q: %v", uuidStr, err)
		}
		return "uuid:" + id.String(), nil
	}
	// create a new uuid otherwise
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate UUID: %w", err)
	}
	return "uuid:" + id.String(), nil
}

func validateTimeToEnd(timeToEndStr string) (time.Time, error) {
	if timeToEndStr == "" {
		return time.Time{}, nil
	}

	now := time.Now()
	parsed, err := time.Parse("15:04", timeToEndStr) // 15:04 is the layout for HH:MM
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format %q (expected HH:MM): %w", timeToEndStr, err)
	}

	// Combine today's date with the parsed hour/min
	result := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())

	// If the time has already passed today, assume they mean tomorrow
	if result.Before(now) {
		result = result.Add(24 * time.Hour)
	}

	return result, nil
}
