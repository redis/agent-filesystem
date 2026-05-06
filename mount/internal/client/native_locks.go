package client

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

type storedFileLock struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
	Type  uint32 `json:"type"`
	PID   uint32 `json:"pid"`
}

func (c *nativeClient) Getlk(ctx context.Context, inode uint64, handleID string, lk *FileLock) (*FileLock, error) {
	if err := validateFileLock(lk); err != nil {
		return nil, err
	}
	locks, err := c.loadLockState(ctx, inode)
	if err != nil {
		return nil, err
	}

	var conflict *FileLock
	for owner, ownerLocks := range locks {
		if owner == handleID {
			continue
		}
		for _, existing := range ownerLocks {
			if !locksConflict(existing, *lk) {
				continue
			}
			if conflict == nil || existing.Start < conflict.Start || (existing.Start == conflict.Start && existing.End < conflict.End) {
				candidate := existing
				conflict = &candidate
			}
		}
	}
	if conflict == nil {
		return &FileLock{Type: syscall.F_UNLCK}, nil
	}
	return conflict, nil
}

func (c *nativeClient) Setlk(ctx context.Context, inode uint64, handleID string, lk *FileLock, wait bool) error {
	if err := validateFileLock(lk); err != nil {
		return err
	}
	if !wait {
		return c.trySetLock(ctx, inode, handleID, lk)
	}

	for {
		err := c.trySetLock(ctx, inode, handleID, lk)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrLockWouldBlock):
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		default:
			return err
		}
	}
}

func (c *nativeClient) UnlockAll(ctx context.Context, inode uint64, handleID string) error {
	lockKey := c.keys.locks(strconv.FormatUint(inode, 10))
	return c.retryWatch(ctx, []string{lockKey}, func(tx *redis.Tx) error {
		if _, err := tx.HGet(ctx, lockKey, handleID).Result(); err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HDel(ctx, lockKey, handleID)
			return nil
		})
		return err
	})
}

func (c *nativeClient) trySetLock(ctx context.Context, inode uint64, handleID string, lk *FileLock) error {
	lockKey := c.keys.locks(strconv.FormatUint(inode, 10))

	return c.retryWatch(ctx, []string{lockKey}, func(tx *redis.Tx) error {
		state, err := loadLockStateFromTx(ctx, tx, lockKey)
		if err != nil {
			return err
		}
		current := state[handleID]

		if lk.Type != syscall.F_UNLCK {
			for owner, ownerLocks := range state {
				if owner == handleID {
					continue
				}
				for _, existing := range ownerLocks {
					if locksConflict(existing, *lk) {
						return ErrLockWouldBlock
					}
				}
			}
		}

		nextLocks := applyFileLock(current, *lk)
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			if len(nextLocks) == 0 {
				pipe.HDel(ctx, lockKey, handleID)
			} else {
				encoded, err := encodeFileLocks(nextLocks)
				if err != nil {
					return err
				}
				pipe.HSet(ctx, lockKey, handleID, encoded)
			}
			return nil
		})
		return err
	})
}

func (c *nativeClient) loadLockState(ctx context.Context, inode uint64) (map[string][]FileLock, error) {
	lockKey := c.keys.locks(strconv.FormatUint(inode, 10))
	values, err := c.rdb.HGetAll(ctx, lockKey).Result()
	if err != nil {
		return nil, err
	}
	return decodeLockState(values)
}

func loadLockStateFromTx(ctx context.Context, tx *redis.Tx, lockKey string) (map[string][]FileLock, error) {
	values, err := tx.HGetAll(ctx, lockKey).Result()
	if err != nil {
		return nil, err
	}
	return decodeLockState(values)
}

func decodeLockState(values map[string]string) (map[string][]FileLock, error) {
	if len(values) == 0 {
		return map[string][]FileLock{}, nil
	}
	out := make(map[string][]FileLock, len(values))
	for handleID, raw := range values {
		var stored []storedFileLock
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return nil, err
		}
		locks := make([]FileLock, 0, len(stored))
		for _, item := range stored {
			locks = append(locks, FileLock{
				Start: item.Start,
				End:   item.End,
				Type:  item.Type,
				PID:   item.PID,
			})
		}
		out[handleID] = normalizeFileLocks(locks)
	}
	return out, nil
}

func encodeFileLocks(locks []FileLock) (string, error) {
	stored := make([]storedFileLock, 0, len(locks))
	for _, lock := range normalizeFileLocks(locks) {
		stored = append(stored, storedFileLock{
			Start: lock.Start,
			End:   lock.End,
			Type:  lock.Type,
			PID:   lock.PID,
		})
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func applyFileLock(existing []FileLock, requested FileLock) []FileLock {
	out := make([]FileLock, 0, len(existing)+1)
	for _, current := range existing {
		if !lockRangesOverlap(current, requested) {
			out = append(out, current)
			continue
		}
		if current.Start < requested.Start {
			out = append(out, FileLock{
				Start: current.Start,
				End:   requested.Start - 1,
				Type:  current.Type,
				PID:   current.PID,
			})
		}
		if requested.End < current.End {
			out = append(out, FileLock{
				Start: requested.End + 1,
				End:   current.End,
				Type:  current.Type,
				PID:   current.PID,
			})
		}
	}
	if requested.Type != syscall.F_UNLCK {
		out = append(out, requested)
	}
	return normalizeFileLocks(out)
}

func normalizeFileLocks(locks []FileLock) []FileLock {
	if len(locks) == 0 {
		return nil
	}
	sort.Slice(locks, func(i, j int) bool {
		if locks[i].Start == locks[j].Start {
			return locks[i].End < locks[j].End
		}
		return locks[i].Start < locks[j].Start
	})

	out := make([]FileLock, 0, len(locks))
	for _, lock := range locks {
		if lock.Type == syscall.F_UNLCK {
			continue
		}
		if len(out) == 0 {
			out = append(out, lock)
			continue
		}
		last := &out[len(out)-1]
		if last.Type == lock.Type && lock.Start <= nextLockStart(last.End) {
			if lock.End > last.End {
				last.End = lock.End
			}
			if lock.PID != 0 {
				last.PID = lock.PID
			}
			continue
		}
		out = append(out, lock)
	}
	return out
}

func validateFileLock(lk *FileLock) error {
	if lk == nil {
		return errors.New("invalid lock")
	}
	switch lk.Type {
	case syscall.F_RDLCK, syscall.F_WRLCK, syscall.F_UNLCK:
	default:
		return errors.New("invalid lock type")
	}
	if lk.End < lk.Start {
		return errors.New("invalid lock range")
	}
	return nil
}

func lockRangesOverlap(a, b FileLock) bool {
	return a.Start <= b.End && b.Start <= a.End
}

func locksConflict(existing, requested FileLock) bool {
	if existing.Type == syscall.F_UNLCK || requested.Type == syscall.F_UNLCK {
		return false
	}
	if !lockRangesOverlap(existing, requested) {
		return false
	}
	return existing.Type == syscall.F_WRLCK || requested.Type == syscall.F_WRLCK
}

func nextLockStart(end uint64) uint64 {
	if end == ^uint64(0) {
		return end
	}
	return end + 1
}
