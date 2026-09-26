package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRobotID   = "default"
	maxRobots        = 32
	maxRobotConfig   = 256 * 1024
	robotHealthy     = "healthy"
	robotDegraded    = "degraded"
	robotUnavailable = "unavailable"
)

var (
	robotIDPattern         = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	ErrRobotNotFound       = errors.New("robot_not_found")
	ErrRobotAuthentication = errors.New("robot_authentication_failed")
	ErrRobotAlreadyExists  = errors.New("robot_already_exists")
	ErrRobotTokenExists    = errors.New("robot_token_exists")
	ErrRobotLimitExceeded  = errors.New("robot_limit_exceeded")
	ErrInvalidRobotConfig  = errors.New("invalid_robot_config")
	ErrRobotCannotRemove   = errors.New("robot_cannot_remove")
)

type RobotConfig struct {
	ID     string
	Name   string
	Token  string
	Groups map[uint64]Target
}

type RobotRegistration struct {
	ID    string
	Name  string
	Token string
}

type RobotStatus struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	State         string        `json:"state"`
	Authenticated bool          `json:"authenticated"`
	Identity      RobotIdentity `json:"identity"`
	LastCheckedAt time.Time     `json:"last_checked_at"`
	ErrorCode     string        `json:"error_code,omitempty"`
	Queue         QueueStats    `json:"queue"`
	Chats         []Chat        `json:"chats"`
	Bindings      []Binding     `json:"bindings"`
}

type fleetRobot struct {
	config  RobotConfig
	client  *PotatoClient
	manager *Manager
	running bool
}

type Fleet struct {
	dir        string
	source     SnapshotSource
	store      *RobotConfigStore
	registered map[string]RobotConfig

	mu      sync.RWMutex
	workers sync.WaitGroup
	runCtx  context.Context
	robots  map[string]*fleetRobot
	order   []string
}

type RobotConfigStore struct {
	path string
	mu   sync.Mutex
}

func NewRobotConfigStore(path string) *RobotConfigStore {
	return &RobotConfigStore{path: path}
}

func ParseRobotConfigs(value, legacyToken, legacyGroups string) ([]RobotConfig, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		targets, err := ParseTargets(legacyToken, legacyGroups)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(legacyToken) == "" {
			return nil, nil
		}
		return []RobotConfig{{
			ID: defaultRobotID, Name: "默认机器人", Token: legacyToken, Groups: targets,
		}}, nil
	}
	if len(value) > maxRobotConfig {
		return nil, errors.New("AGP_POTATO_ROBOTS is too large")
	}
	var raw map[string]struct {
		Name   string            `json:"name"`
		Token  string            `json:"token"`
		Groups map[string]Target `json:"groups"`
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil || raw == nil {
		return nil, errors.New("invalid AGP_POTATO_ROBOTS JSON")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid AGP_POTATO_ROBOTS JSON")
	}
	if len(raw) == 0 || len(raw) > maxRobots {
		return nil, fmt.Errorf("AGP_POTATO_ROBOTS requires 1-%d robots", maxRobots)
	}

	configs := make([]RobotConfig, 0, len(raw))
	tokens := make(map[string]struct{}, len(raw))
	for id, item := range raw {
		config, err := validateRobotConfig(id, item.Name, item.Token, "AGP_POTATO_ROBOTS")
		if err != nil {
			return nil, err
		}
		if _, exists := tokens[config.Token]; exists {
			return nil, errors.New("AGP_POTATO_ROBOTS contains a duplicate token")
		}
		tokens[config.Token] = struct{}{}
		targets, err := parseRobotTargets(item.Groups)
		if err != nil {
			return nil, err
		}
		config.Groups = targets
		configs = append(configs, config)
	}
	sortRobotConfigs(configs)
	return configs, nil
}

func (s *RobotConfigStore) Load() ([]RobotConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, fmt.Errorf("create robot config directory: %w", err)
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read robot config: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return nil, fmt.Errorf("secure robot config: %w", err)
	}
	var document struct {
		Robots []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Token string `json:"token"`
		} `json:"robots"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode robot config: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode robot config: trailing content")
	}
	configs := make([]RobotConfig, 0, len(document.Robots))
	ids := make(map[string]struct{}, len(document.Robots))
	tokens := make(map[string]struct{}, len(document.Robots))
	for _, item := range document.Robots {
		config, err := validateRobotConfig(item.ID, item.Name, item.Token, "stored robot config")
		if err != nil {
			return nil, err
		}
		if _, exists := ids[config.ID]; exists {
			return nil, errors.New("stored robot config contains a duplicate robot ID")
		}
		if _, exists := tokens[config.Token]; exists {
			return nil, errors.New("stored robot config contains a duplicate token")
		}
		ids[config.ID] = struct{}{}
		tokens[config.Token] = struct{}{}
		configs = append(configs, config)
	}
	sortRobotConfigs(configs)
	return configs, nil
}

func (s *RobotConfigStore) Save(configs []RobotConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create robot config directory: %w", err)
	}
	configs = append([]RobotConfig(nil), configs...)
	sortRobotConfigs(configs)
	document := struct {
		Robots []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Token string `json:"token"`
		} `json:"robots"`
	}{Robots: make([]struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}, 0, len(configs))}
	for _, config := range configs {
		document.Robots = append(document.Robots, struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Token string `json:"token"`
		}{ID: config.ID, Name: config.Name, Token: config.Token})
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode robot config: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(s.path, data); err != nil {
		return fmt.Errorf("write robot config: %w", err)
	}
	return nil
}

func validateRobotConfig(id, name, token, source string) (RobotConfig, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	token = strings.TrimSpace(token)
	if !robotIDPattern.MatchString(id) {
		return RobotConfig{}, fmt.Errorf("%s contains an invalid robot ID", source)
	}
	if name == "" || len([]rune(name)) > 128 {
		return RobotConfig{}, fmt.Errorf("%s requires robot names of 1-128 characters", source)
	}
	if !tokenPattern.MatchString(token) {
		return RobotConfig{}, fmt.Errorf("%s contains an invalid token", source)
	}
	return RobotConfig{ID: id, Name: name, Token: token}, nil
}

func sortRobotConfigs(configs []RobotConfig) {
	sort.Slice(configs, func(i, j int) bool { return configs[i].ID < configs[j].ID })
}

func parseRobotTargets(raw map[string]Target) (map[uint64]Target, error) {
	targets := make(map[uint64]Target, len(raw))
	chatIDs := make(map[int64]struct{}, len(raw))
	for key, target := range raw {
		groupID, err := strconv.ParseUint(key, 10, 64)
		if err != nil || groupID == 0 || strconv.FormatUint(groupID, 10) != key ||
			target.ChatID <= 0 || (target.ChatType != 2 && target.ChatType != 3) {
			return nil, errors.New("AGP_POTATO_ROBOTS groups require positive IDs and chat_type 2 or 3")
		}
		if _, exists := chatIDs[target.ChatID]; exists {
			return nil, errors.New("AGP_POTATO_ROBOTS assigns a robot chat more than once")
		}
		chatIDs[target.ChatID] = struct{}{}
		targets[groupID] = target
	}
	return targets, nil
}

func NewFleet(dir string, configs []RobotConfig, source SnapshotSource) (*Fleet, error) {
	store := NewRobotConfigStore(filepath.Join(dir, "robots.json"))
	storedConfigs, err := store.Load()
	if err != nil {
		return nil, err
	}
	merged, registered, err := mergeRobotConfigs(configs, storedConfigs)
	if err != nil {
		return nil, err
	}
	fleet := &Fleet{
		dir:        dir,
		source:     source,
		store:      store,
		registered: registered,
		robots:     make(map[string]*fleetRobot, len(merged)),
	}
	for _, config := range merged {
		robot, err := newFleetRobot(dir, source, config)
		if err != nil {
			return nil, err
		}
		fleet.robots[config.ID] = robot
		fleet.order = append(fleet.order, config.ID)
	}
	sort.Strings(fleet.order)
	return fleet, nil
}

func mergeRobotConfigs(configured, stored []RobotConfig) ([]RobotConfig, map[string]RobotConfig, error) {
	merged := make([]RobotConfig, 0, len(configured)+len(stored))
	registered := make(map[string]RobotConfig, len(stored))
	ids := make(map[string]struct{}, len(configured)+len(stored))
	tokens := make(map[string]struct{}, len(configured)+len(stored))
	for _, config := range configured {
		if _, exists := ids[config.ID]; exists {
			return nil, nil, errors.New("robot configuration contains a duplicate robot ID")
		}
		if _, exists := tokens[config.Token]; exists {
			return nil, nil, errors.New("robot configuration contains a duplicate token")
		}
		ids[config.ID] = struct{}{}
		tokens[config.Token] = struct{}{}
		merged = append(merged, config)
	}
	for _, config := range stored {
		if _, exists := ids[config.ID]; exists {
			continue
		}
		if _, exists := tokens[config.Token]; exists {
			return nil, nil, errors.New("stored robot config conflicts with deployment robot token")
		}
		ids[config.ID] = struct{}{}
		tokens[config.Token] = struct{}{}
		registered[config.ID] = config
		merged = append(merged, config)
	}
	sortRobotConfigs(merged)
	return merged, registered, nil
}

func newFleetRobot(dir string, source SnapshotSource, config RobotConfig) (*fleetRobot, error) {
	client, err := NewPotatoClient(config.Token)
	if err != nil {
		return nil, fmt.Errorf("register robot %s: %w", config.ID, err)
	}
	robotDir := filepath.Join(dir, "robots", config.ID)
	if config.ID == defaultRobotID {
		robotDir = dir
	}
	manager, err := NewManager(robotDir, config.Groups, source, client)
	if err != nil {
		return nil, fmt.Errorf("register robot %s: %w", config.ID, err)
	}
	return &fleetRobot{config: config, client: client, manager: manager}, nil
}

func (f *Fleet) Run(ctx context.Context) {
	f.mu.Lock()
	f.runCtx = ctx
	for _, id := range f.order {
		f.startRobotLocked(f.robots[id])
	}
	f.mu.Unlock()
	<-ctx.Done()
	f.workers.Wait()
}

func (f *Fleet) startRobotLocked(robot *fleetRobot) {
	if robot == nil || robot.running || f.runCtx == nil || f.runCtx.Err() != nil {
		return
	}
	robot.running = true
	f.workers.Go(func() { robot.manager.Run(f.runCtx) })
}

func (f *Fleet) robotList() []*fleetRobot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	robots := make([]*fleetRobot, 0, len(f.order))
	for _, id := range f.order {
		robots = append(robots, f.robots[id])
	}
	return robots
}

func (f *Fleet) Enqueue(event Event) error {
	var errs []error
	for _, robot := range f.robotList() {
		if err := robot.manager.Enqueue(event); err != nil {
			errs = append(errs, fmt.Errorf("robot %s: %w", robot.config.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (f *Fleet) EnqueueInitial(now time.Time) error {
	var errs []error
	for _, robot := range f.robotList() {
		if err := robot.manager.EnqueueInitial(now); err != nil {
			errs = append(errs, fmt.Errorf("robot %s: %w", robot.config.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (f *Fleet) WakeInitial(groupID uint64, now time.Time) error {
	var errs []error
	for _, robot := range f.robotList() {
		if err := robot.manager.WakeInitial(groupID, now); err != nil {
			errs = append(errs, fmt.Errorf("robot %s: %w", robot.config.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (f *Fleet) Robots(ctx context.Context) []RobotStatus {
	robots := f.robotList()
	statuses := make([]RobotStatus, len(robots))
	var workers sync.WaitGroup
	for index, robot := range robots {
		workers.Go(func() {
			statuses[index] = robot.status(ctx)
		})
	}
	workers.Wait()
	return statuses
}

func (r *fleetRobot) status(ctx context.Context) RobotStatus {
	status := RobotStatus{
		ID:            r.config.ID,
		Name:          r.config.Name,
		State:         robotHealthy,
		LastCheckedAt: time.Now().UTC(),
		Bindings:      r.manager.Bindings(),
	}
	status.Chats = chatsFromBindings(status.Bindings)
	identity, err := r.client.Identity(ctx)
	if err != nil {
		status.State = robotUnavailable
		status.ErrorCode = "authentication_failed"
	} else {
		status.Authenticated = true
		status.Identity = identity
		chats, err := r.manager.Chats(ctx)
		if err != nil {
			status.State = robotDegraded
			status.ErrorCode = "chat_list_failed"
		} else {
			boundGroups := make(map[int64]uint64, len(status.Bindings))
			for _, binding := range status.Bindings {
				boundGroups[binding.ChatID] = binding.GroupID
			}
			for index := range chats {
				chats[index].GroupID = boundGroups[chats[index].ChatID]
				delete(boundGroups, chats[index].ChatID)
			}
			for _, binding := range status.Bindings {
				if _, missing := boundGroups[binding.ChatID]; missing {
					chats = append(chats, Chat{
						ChatID: binding.ChatID, ChatType: binding.ChatType,
						Title: "已绑定群聊", GroupID: binding.GroupID,
					})
				}
			}
			status.Chats = chats
		}
	}
	stats, err := r.manager.Stats()
	if err != nil {
		if status.State == robotHealthy {
			status.State = robotDegraded
			status.ErrorCode = "queue_status_failed"
		}
	} else {
		status.Queue = stats
	}
	return status
}

func chatsFromBindings(bindings []Binding) []Chat {
	chats := make([]Chat, 0, len(bindings))
	for _, binding := range bindings {
		chats = append(chats, Chat{
			ChatID: binding.ChatID, ChatType: binding.ChatType,
			Title: "已绑定群聊", GroupID: binding.GroupID,
		})
	}
	return chats
}

func (f *Fleet) Assign(
	ctx context.Context,
	robotID string,
	target Target,
	groupID uint64,
	now time.Time,
) error {
	if strings.TrimSpace(robotID) == "" {
		robotID = defaultRobotID
	}
	f.mu.RLock()
	robot, ok := f.robots[robotID]
	f.mu.RUnlock()
	if !ok {
		return ErrRobotNotFound
	}
	if groupID > 0 {
		if _, err := robot.client.Identity(ctx); err != nil {
			return ErrRobotAuthentication
		}
	}
	return robot.manager.Assign(ctx, target, groupID, now)
}

func (f *Fleet) BindingGroupID(robotID string, chatID int64) uint64 {
	if strings.TrimSpace(robotID) == "" {
		robotID = defaultRobotID
	}
	f.mu.RLock()
	robot, ok := f.robots[robotID]
	f.mu.RUnlock()
	if !ok {
		return 0
	}
	for _, binding := range robot.manager.Bindings() {
		if binding.ChatID == chatID {
			return binding.GroupID
		}
	}
	return 0
}

func (f *Fleet) Register(ctx context.Context, req RobotRegistration) (RobotStatus, error) {
	req.Token = strings.TrimSpace(req.Token)
	if !tokenPattern.MatchString(req.Token) {
		return RobotStatus{}, ErrInvalidRobotConfig
	}
	client, err := NewPotatoClient(req.Token)
	if err != nil {
		return RobotStatus{}, ErrInvalidRobotConfig
	}
	identity, err := client.Identity(ctx)
	if err != nil {
		return RobotStatus{}, ErrRobotAuthentication
	}
	config, err := robotConfigFromRegistration(req, identity)
	if err != nil {
		return RobotStatus{}, err
	}
	f.mu.RLock()
	if len(f.robots) >= maxRobots {
		f.mu.RUnlock()
		return RobotStatus{}, ErrRobotLimitExceeded
	}
	if _, exists := f.robots[config.ID]; exists {
		f.mu.RUnlock()
		return RobotStatus{}, ErrRobotAlreadyExists
	}
	for _, existing := range f.robots {
		if existing.config.Token == config.Token {
			f.mu.RUnlock()
			return RobotStatus{}, ErrRobotTokenExists
		}
	}
	f.mu.RUnlock()

	robot, err := newFleetRobot(f.dir, f.source, config)
	if err != nil {
		return RobotStatus{}, err
	}
	robot.client = client
	robot.manager.client = client

	f.mu.Lock()
	if len(f.robots) >= maxRobots {
		f.mu.Unlock()
		return RobotStatus{}, ErrRobotLimitExceeded
	}
	if _, exists := f.robots[config.ID]; exists {
		f.mu.Unlock()
		return RobotStatus{}, ErrRobotAlreadyExists
	}
	for _, existing := range f.robots {
		if existing.config.Token == config.Token {
			f.mu.Unlock()
			return RobotStatus{}, ErrRobotTokenExists
		}
	}
	nextRegistered := make(map[string]RobotConfig, len(f.registered)+1)
	for id, registered := range f.registered {
		nextRegistered[id] = registered
	}
	nextRegistered[config.ID] = config
	if err := f.store.Save(configsFromMap(nextRegistered)); err != nil {
		f.mu.Unlock()
		return RobotStatus{}, err
	}
	f.registered = nextRegistered
	f.robots[config.ID] = robot
	f.order = append(f.order, config.ID)
	sort.Strings(f.order)
	f.startRobotLocked(robot)
	f.mu.Unlock()
	return robot.status(ctx), nil
}

// Remove unregisters a robot added through the admin API.
func (f *Fleet) Remove(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || id == defaultRobotID { return ErrRobotCannotRemove }
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.robots[id]; !ok { return ErrRobotNotFound }
	next := make(map[string]RobotConfig, len(f.registered))
	for key, config := range f.registered { if key != id { next[key] = config } }
	if err := f.store.Save(configsFromMap(next)); err != nil { return err }
	f.registered = next
	delete(f.robots, id)
	for i, key := range f.order { if key == id { f.order = append(f.order[:i], f.order[i+1:]...); break } }
	return nil
}

func robotConfigFromRegistration(req RobotRegistration, identity RobotIdentity) (RobotConfig, error) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = robotIDFromIdentity(identity)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = firstNonEmptyString(identity.FirstName, identity.Username, id)
	}
	config, err := validateRobotConfig(id, name, req.Token, "robot registration")
	if err != nil {
		return RobotConfig{}, ErrInvalidRobotConfig
	}
	return config, nil
}

func robotIDFromIdentity(identity RobotIdentity) string {
	source := strings.ToLower(strings.TrimSpace(identity.Username))
	var builder strings.Builder
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	id := strings.Trim(builder.String(), "-_")
	if id == "" || id[0] < 'a' || id[0] > 'z' {
		id = "bot-" + id
	}
	if len(id) > 32 {
		id = strings.Trim(id[:32], "-_")
	}
	if id == "" {
		id = "bot"
	}
	if !robotIDPattern.MatchString(id) {
		return "bot"
	}
	return id
}

func configsFromMap(configs map[string]RobotConfig) []RobotConfig {
	items := make([]RobotConfig, 0, len(configs))
	for _, config := range configs {
		items = append(items, config)
	}
	sortRobotConfigs(items)
	return items
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
