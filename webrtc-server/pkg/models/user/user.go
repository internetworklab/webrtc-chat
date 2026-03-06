package user

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/google/uuid"
)

// ErrUsernameDuplicated is returned when attempting to create a user with a username that already exists.
var ErrUsernameDuplicated = errors.New("username already exists")

// `User`s are immutable, no one should modify a `User` returned from a `UserManager`.
type User struct {
	Id          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	GithubId    string `json:"github_id"`
}

type UserManager interface {
	// returns (theUser, created, error)
	LoadOrCreateNewUserByGithubId(ctx context.Context, githubId string, newUser User) (*User, bool, error)

	GetUserById(ctx context.Context, userId string) (*User, error)

	GetUserByUsername(ctx context.Context, username string) (*User, error)
}

type InMemoryUserStore struct {
	Users           []User
	IndexById       map[string]int
	IndexByGithubId map[string]int
	IndexByUsername map[string]int
}

// Only call this after indexes were cloned, make sure you are modifying a memory region that
// is private to current goroutine
func (newStore *InMemoryUserStore) updateIndex(u *User, i int) {
	newStore.IndexById[u.Id] = i
	newStore.IndexByGithubId[u.GithubId] = i
	newStore.IndexByUsername[u.Username] = i
}

func (store *InMemoryUserStore) Clone() *InMemoryUserStore {
	// Each new Store would be private to current goroutine, so
	// there will NEVER be a concurrent write!
	newStore := new(InMemoryUserStore)
	newStore.IndexById = make(map[string]int)
	newStore.IndexByGithubId = make(map[string]int)
	newStore.IndexByUsername = make(map[string]int)
	if store != nil {
		*newStore = *store
		newStore.Users = make([]User, len(store.Users))
		for i := range store.Users {
			u := store.Users[i]
			newStore.Users[i] = u
			newStore.updateIndex(&u, i)
		}
	}
	return newStore
}

func (store *InMemoryUserStore) AddUser(user User) (*InMemoryUserStore, error) {
	newStore := store.Clone()

	// NOTE: after Clone(), each thread modififies the clone of its own, not the same memory region

	if _, exists := newStore.IndexByUsername[user.Username]; exists {
		return nil, ErrUsernameDuplicated
	}

	numId := len(newStore.Users)
	newStore.updateIndex(&user, numId)
	newStore.Users = append(newStore.Users, user)
	return newStore, nil
}

type MemoryUserManager struct {
	store atomic.Pointer[InMemoryUserStore]
}

// Returns true means new user is created
func (memUserMngr *MemoryUserManager) doAddUserByGithubId(user User) (*User, bool, error) {
	for {
		oldStore := memUserMngr.store.Load()
		if oldStore != nil {
			if idx, hit := oldStore.IndexByGithubId[user.GithubId]; hit {
				return &oldStore.Users[idx], false, nil
			}
		}
		newStore, err := oldStore.AddUser(user)
		if err != nil {
			return nil, false, err
		}
		if memUserMngr.store.CompareAndSwap(oldStore, newStore) {
			return &user, true, nil
		}
	}
}

func (memUserMngr *MemoryUserManager) LoadOrCreateNewUserByGithubId(ctx context.Context, githubId string, newUser User) (*User, bool, error) {

	if id := newUser.Id; id == "" {
		newUser.Id = uuid.NewString()
	}

	u, accepted, err := memUserMngr.doAddUserByGithubId(newUser)
	if err != nil {
		return nil, false, err
	}
	return u, accepted, nil
}

func (memUserMngr *MemoryUserManager) GetUserById(ctx context.Context, userId string) (*User, error) {
	if store := memUserMngr.store.Load(); store != nil {
		if idx, hit := store.IndexById[userId]; hit {
			return &store.Users[idx], nil
		}
	}
	return nil, nil
}

func (memUserMngr *MemoryUserManager) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	if store := memUserMngr.store.Load(); store != nil {
		if idx, hit := store.IndexByUsername[username]; hit {
			return &store.Users[idx], nil
		}
	}
	return nil, nil
}
