package sponsor

// LICENSE

// Copyright (c) evcc.io (andig, naltatis, premultiply)

// This module is NOT covered by the MIT license. All rights reserved.

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/evcc-io/evcc/util/machine"
)

var (
	mu                            sync.RWMutex
	Subject, Token, ActivationKey string
	ExpiresAt                     time.Time
)

func machineID() string {
	return machine.ProtectedID("evcc-sponsor")
}

const unavailable = "sponsorship unavailable"
const defaultSubject = "community"

func IsAuthorized() bool {
	return true
}

func IsAuthorizedForApi() bool {
	return true
}

// ActivateSponsorship activates a license key with email and returns the JWT token
func ActivateSponsorship(licenseKey, _ string) (string, error) {
	return licenseKey, nil
}

// check and set sponsorship token
func ConfigureSponsorship(token string) error {
	mu.Lock()
	defer mu.Unlock()

	Subject = ""
	if token == "" {
		if sub := checkVictron(); sub != "" && sub != unavailable {
			Subject = sub
		}

		if Subject == "" && os.Getenv("HEMSPRO") != "" {
			if sub := checkHemsPro(); sub != "" && sub != unavailable {
				Subject = sub
			}
		}

		if autoToken, err := checkPulsares(); err == nil && autoToken != "" {
			token = autoToken
		}
	}

	Token = token
	if Subject == "" {
		Subject = defaultSubject
	}
	ActivationKey = ""
	ExpiresAt = time.Time{}
	return nil
}

// redactToken returns a redacted version of the token showing only start and end characters
func redactToken(token string) string {
	if len(token) <= 12 {
		return ""
	}
	return token[:6] + "......." + token[len(token)-6:]
}

// redactKey returns a redacted version of the activation key showing only the first segment
func redactKey(key string) string {
	if idx := strings.Index(key, "-"); idx > 0 {
		return key[:idx] + "-XXXXX-XXXXX-XXXXX-XXXXX"
	}
	return ""
}

type Status struct {
	Name          string    `json:"name"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
	ExpiresSoon   bool      `json:"expiresSoon,omitempty"`
	Token         string    `json:"token,omitempty"`
	ActivationKey string    `json:"activationKey,omitempty"`
}

// RedactedStatus returns the sponsorship status
func RedactedStatus() Status {
	mu.RLock()
	defer mu.RUnlock()

	var expiresSoon bool
	if d := time.Until(ExpiresAt); d < 30*24*time.Hour && d > 0 {
		expiresSoon = true
	}

	return Status{
		Name:          Subject,
		ExpiresAt:     ExpiresAt,
		ExpiresSoon:   expiresSoon,
		Token:         redactToken(Token),
		ActivationKey: redactKey(ActivationKey),
	}
}
