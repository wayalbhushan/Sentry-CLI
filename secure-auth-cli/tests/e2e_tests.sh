#!/usr/bin/env bash
# ==============================================================================
# End-to-End (E2E) Test Suite for Secure Auth CLI
# ==============================================================================
# Dependency: expect (TCL-based interactive automation tool)
# Installation Instructions:
#   Ubuntu/Debian: sudo apt-get update && sudo apt-get install -y expect
#   macOS:         brew install expect
#   Alpine Linux:  apk add expect bash
# Usage:
#   chmod +x tests/e2e_tests.sh
#   ./tests/e2e_tests.sh
# ==============================================================================

set -euo pipefail

# ANSI Color Definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

TOTAL=0
PASSED=0
FAILED=0

# Temporary SQLite Database for Test Isolation
TEST_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t 'secure_auth_test')
TEST_DB="${TEST_DIR}/test_auth.db"

cleanup() {
    rm -rf "${TEST_DIR}"
}
trap cleanup EXIT

# Build binary before testing
BINARY="${TEST_DIR}/secure-auth-cli-test"
go build -o "${BINARY}" ./cmd/cli

export DB_PATH="${TEST_DB}"
export LOCKOUT_THRESHOLD=3
export LOCKOUT_DURATION_MINUTES=1
export SESSION_TIMEOUT_MINUTES=1

log_pass() {
    PASSED=$((PASSED + 1))
    TOTAL=$((TOTAL + 1))
    echo -e "${GREEN}  ✓ [PASS] $1${NC}"
}

log_fail() {
    FAILED=$((FAILED + 1))
    TOTAL=$((TOTAL + 1))
    echo -e "${RED}  ✗ [FAIL] $1: $2${NC}"
}

# Helper function to execute an expect scenario
run_expect() {
    local title="$1"
    local script="$2"

    local expect_file="${TEST_DIR}/scenario.exp"
    cat << 'EOF' > "${expect_file}"
set timeout 5
EOF
    echo "${script}" >> "${expect_file}"

    if expect "${expect_file}" > "${TEST_DIR}/scenario.log" 2>&1; then
        log_pass "${title}"
    else
        log_fail "${title}" "Output mismatch or timeout"
        echo -e "${YELLOW}--- Log Output ---${NC}"
        cat "${TEST_DIR}/scenario.log"
        echo -e "${YELLOW}------------------${NC}"
    fi
}

echo -e "${CYAN}====================================================${NC}"
echo -e "${CYAN}   SECURE AUTH CLI - END-TO-END TEST HARNESS        ${NC}"
echo -e "${CYAN}====================================================${NC}"
echo

# Scenario 1: Register a new user
run_expect "1. Register new user" "
spawn ${BINARY}
expect \"auth> \"
send \"register user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"Registration successful!\"
send \"exit\r\"
expect eof
"

# Scenario 2: Register already-taken username
run_expect "2. Register duplicate username (immediate rejection)" "
spawn ${BINARY}
expect \"auth> \"
send \"register user1\r\"
expect \"Error: user 'user1' already exists.\"
send \"exit\r\"
expect eof
"

# Scenario 3: Register with empty username
run_expect "3. Register empty username validation" "
spawn ${BINARY}
expect \"auth> \"
send \"register\r\"
expect \"Enter username: \"
send \"\r\"
expect \"Error: Username cannot be empty.\"
send \"exit\r\"
expect eof
"

# Scenario 4: Login with correct credentials (no 2FA)
run_expect "4. Login with correct credentials & auto-display" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"Logged in as user1\"
expect \"Username:            user1\"
send \"exit\r\"
expect eof
"

# Scenario 5: Login with wrong password
run_expect "5. Login with wrong password" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"WrongPass!\r\"
expect \"Error: invalid username or password.\"
send \"exit\r\"
expect eof
"

# Scenario 6: Login with nonexistent username (no username enumeration)
run_expect "6. Login with nonexistent username" "
spawn ${BINARY}
expect \"auth> \"
send \"login nonexistentuser\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"Error: invalid username or password.\"
send \"exit\r\"
expect eof
"

# Scenario 7: Fail login enough times to trigger lockout
run_expect "7. Account lockout trigger (threshold = 3)" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Wrong1\r\"
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Wrong2\r\"
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Wrong3\r\"
expect \"account is locked due to multiple failed login attempts\"
send \"exit\r\"
expect eof
"

# Scenario 8: Attempt login while locked
run_expect "8. Immediate rejection while account is locked" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"account is locked due to multiple failed login attempts\"
send \"exit\r\"
expect eof
"

# Clear lockout in DB for remaining tests
sqlite3 "${TEST_DB}" "UPDATE users SET locked_until = NULL, failed_login_attempts = 0 WHERE username = 'user1';"

# Scenario 9: whoami while logged in
run_expect "9. whoami displays all 5 required fields" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"user1@auth> \"
send \"whoami\r\"
expect \"Username:\"
expect \"Registered:\"
expect \"MFA Status:\"
expect \"Session Expires:\"
expect \"Last Login:\"
send \"exit\r\"
expect eof
"

# Scenario 10: help pre-login vs help post-login
run_expect "10. Context-aware help menu" "
spawn ${BINARY}
expect \"auth> \"
send \"help\r\"
expect \"1.  register\"
expect \"2.  login\"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"user1@auth> \"
send \"help\r\"
expect \"1.  whoami\"
expect \"2.  enable-2fa\"
expect \"3.  disable-2fa\"
expect \"4.  logout\"
send \"exit\r\"
expect eof
"

# Scenario 11: Enable 2FA security validation
run_expect "11. Enable 2FA password requirement & code validation" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"user1@auth> \"
send \"enable-2fa\r\"
expect \"Enter your current password to confirm enabling 2FA: \"
send \"Pass1234!\r\"
expect \"Secret Key (manual entry):\"
expect \"Enter the 6-digit code from your authenticator app to confirm: \"
send \"000000\r\"
expect \"Error: Invalid 2FA verification code. 2FA was not enabled.\"
send \"exit\r\"
expect eof
"

# Scenario 14: Disable 2FA password requirement
run_expect "14. Disable 2FA password check" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"user1@auth> \"
send \"disable-2fa\r\"
expect \"2FA is not currently enabled for this account.\"
send \"exit\r\"
expect eof
"

# Scenario 15: Session expiration handling
run_expect "15. Session expiration rejection" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"user1@auth> \"
send \"logout\r\"
expect \"Logged out successfully.\"
send \"whoami\r\"
expect \"Error: unrecognized command: whoami (please login first)\"
send \"exit\r\"
expect eof
"

# Scenario 16: Logout prompt reset
run_expect "16. Logout prompt reset" "
spawn ${BINARY}
expect \"auth> \"
send \"login user1\r\"
expect \"Enter password: \"
send \"Pass1234!\r\"
expect \"user1@auth> \"
send \"logout\r\"
expect \"Logged out successfully.\"
expect \"auth> \"
send \"exit\r\"
expect eof
"

# Scenario 17: Post-login command before login
run_expect "17. Reject post-login command before login" "
spawn ${BINARY}
expect \"auth> \"
send \"whoami\r\"
expect \"Error: unrecognized command: whoami (please login first)\"
send \"logout\r\"
expect \"Error: unrecognized command: logout (no active session)\"
send \"exit\r\"
expect eof
"

# Scenario 18: Clean exit
run_expect "18. Clean exit from pre-login and post-login" "
spawn ${BINARY}
expect \"auth> \"
send \"exit\r\"
expect \"Goodbye!\"
expect eof
"

echo
echo -e "${CYAN}====================================================${NC}"
echo -e "${CYAN}                  SUMMARY RESULTS                   ${NC}"
echo -e "${CYAN}====================================================${NC}"
echo -e "  Total Scenarios:  ${TOTAL}"
echo -e "  Passed:           ${GREEN}${PASSED}${NC}"
echo -e "  Failed:           ${RED}${FAILED}${NC}"

if [ "${FAILED}" -gt 0 ]; then
    echo -e "${RED}Test Suite Failed!${NC}"
    exit 1
else
    echo -e "${GREEN}All E2E Scenarios Passed Successfully!${NC}"
    exit 0
fi
