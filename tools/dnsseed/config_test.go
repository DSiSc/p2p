package main

import (
	"github.com/DSiSc/craft/log"
	"github.com/DSiSc/monkey"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_NewNodeConfig(t *testing.T) {
	monkey.Patch(GetLogSetting, func(*viper.Viper) log.Config {
		return log.Config{}
	})
	assert := assert.New(t)
	nodeConf := NewNodeConfig()
	assert.NotNil(nodeConf)
	assert.NotNil(nodeConf.Logger)
	monkey.UnpatchAll()
}
