package merchantcatalogai

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var attributeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
