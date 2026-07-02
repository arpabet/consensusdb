/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: Apache-2.0
 */

package constants

type AppInfo struct {
	Version   string
	Build     string
}

var appInfo AppInfo

func SetAppInfo(version, build string) {
	appInfo.Version = version
	appInfo.Build = build
}

func GetAppInfo() *AppInfo {
	return &appInfo
}
