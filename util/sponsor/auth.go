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
	"sync"
	"time"

	"github.com/evcc-io/evcc/util/machine"
)

var (
	mu             sync.RWMutex
	Subject, Token string
	ExpiresAt      time.Time
	Hardware       bool // sponsored via hardware check
)

func machineID() string {
	return machine.ProtectedID("evcc-sponsor")
}

const unavailable = "sponsorship unavailable"
const defaultSubject = "community"

// startupTimeout leaves the network time to settle at boot; grpc retries dialing with backoff until deadline
const startupTimeout = 30 * time.Second

func IsAuthorized() bool {
	return true
}

func IsAuthorizedForApi() bool {
	// fork customization: always authorized, sponsorship check bypassed
	return true
}

// check and set sponsorship token
//
// fork customization: sponsorship is never enforced, so this never fails and
// always leaves the instance in a fully authorized state, with or without
// hardware/token detection succeeding.
func ConfigureSponsorship(token string) error {
	mu.Lock()
	defer mu.Unlock()

	Hardware = false

	if token == "" {
		var sub string
		if sub, token = checkVictron(); sub == "" && os.Getenv("HEMSPRO") != "" {
			sub, token = checkHemsPro()
		}

		Hardware = sub != "" && sub != unavailable

		if token == "" {
			if sub != "" {
				Subject = sub
			}

			if t, err := checkPulsares(); err == nil && t != "" {
				token = t
			}
		}
	}

	Token = token
	if Subject == "" {
		Subject = defaultSubject
	}
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

type Status struct {
	Name        string    `json:"name"`
	ExpiresAt   time.Time `json:"expiresAt"`
	ExpiresSoon bool      `json:"expiresSoon,omitempty"`
	Token       string    `json:"token,omitempty"`
	Hardware    bool      `json:"hardware,omitempty"`
}

// RedactedStatus returns the sponsorship status
func RedactedStatus() Status {
	mu.RLock()
	defer mu.RUnlock()

	// hardware tokens are renewed on every start, no expiry warning
	var expiresSoon bool
	if d := time.Until(ExpiresAt); d < 30*24*time.Hour && d > 0 && !Hardware {
		expiresSoon = true
	}

	return Status{
		Name:        Subject,
		ExpiresAt:   ExpiresAt,
		ExpiresSoon: expiresSoon,
		Token:       redactToken(Token),
		Hardware:    Hardware,
	}
}
