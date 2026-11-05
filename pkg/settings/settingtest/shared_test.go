package settingtest

import (
	"reflect"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func TestEverySharedSettingIsAccountedFor(t *testing.T) {
	Shared.Accounts(t, reflect.TypeFor[schema.Shared]())
}
