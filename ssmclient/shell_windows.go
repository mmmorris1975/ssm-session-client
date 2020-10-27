// +build windows

package ssmclient

import "errors"

func initialize(c datachannel.DataChannel) error {
	// todo
	//  - interrogate terminal size and call updateTermSize()
	//  - setup stdin so that it behaves as expected
	//  - signal handling?
	return nil
}

func cleanup() error {
	// todo - reset stdin to original settings
	return nil
}

func getWinSize() (rows, cols uint32, err error) {
	return 0, 0, errors.New("TODO - not implemented")
}
