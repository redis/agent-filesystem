package main

import (
	"errors"
	"os"
	"path/filepath"
)

func cmdReset() error {
	if st, err := loadState(); err == nil {
		if st.MountPID > 0 || st.SyncPID > 0 {
			if err := unmountAllActive(false); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	removedConfig := false
	if err := os.Remove(configPath()); err == nil {
		removedConfig = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	removedState := false
	if err := os.RemoveAll(stateDir()); err != nil {
		return err
	}
	if _, err := os.Stat(stateDir()); errors.Is(err, os.ErrNotExist) {
		removedState = true
	}

	rows := []outputRow{
		{Label: "config", Value: ternaryString(removedConfig, compactDisplayPath(configPath()), "already clear")},
		{Label: "state", Value: ternaryString(removedState, filepath.Base(stateDir()), "already clear")},
		{Label: "next", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" setup")},
	}
	printSection(markerSuccess+" "+clr(ansiBold, "local state reset"), rows)
	return nil
}
