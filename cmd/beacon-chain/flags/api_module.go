package flags

import "strings"

const QrysmAPIModule string = "qrysm"
const ZondAPIModule string = "zond"

func EnableHTTPQrysmAPI(httpModules string) bool {
	return enableAPI(httpModules, QrysmAPIModule)
}

func EnableHTTPZondAPI(httpModules string) bool {
	return enableAPI(httpModules, ZondAPIModule)
}

func enableAPI(httpModules, api string) bool {
	for _, m := range strings.Split(httpModules, ",") {
		if strings.EqualFold(m, api) {
			return true
		}
	}
	return false
}
