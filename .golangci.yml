version: '2'

run:
  relative-path-mode:
    gitroot
    # Default: false
  allow-parallel-runners: true
  # Allow multiple golangci-lint instances running, but serialize them around a lock.
  # If false, golangci-lint exits with an error if it fails to acquire file lock on start.
  # Default: false
  allow-serial-runners: true
  # Define the Go version limit.
  # Default: use Go version from the go.mod file, fallback on the env var `GOVERSION`, fallback on 1.22.
  go: '1.25'
  # Number of operating system threads (`GOMAXPROCS`) that can execute golangci-lint simultaneously.
  # Default: 0 (automatically set to match Linux container CPU quota and
  # fall back to the number of logical CPUs in the machine)
  concurrency: 8

linters:
  # v2 style: choose the default set; keep "all" and tune via settings/deny rules.
  # See v2 notes and new structure.
  default: all

  # Optionally ensure critical linters are explicitly on (redundant if default: all).
  disable:
    - nonamedreturns
    - wsl

  settings:
    wsl_v5:
      default: all
      # Enabled checks.
      # Will be additive to any presets.
      # https://github.com/bombsimon/wsl/tree/main?tab=readme-ov-file#checks-and-configuration
      # Default: []
      enable:
        - assign
        - branch
        - decl
        - defer
        - expr
        - for
        - go
        - if
        - inc-dec
        - label
        - range
        - return
        - select
        - send
        - switch
        - type-switch
        - append
        - assign-exclusive
        - assign-expr
        - err
        - leading-whitespace
        - trailing-whitespace
    whitespace:
      # Enforces newlines (or comments) after every multi-line if statement.
      # Default: false
      multi-if: true
      # Enforces newlines (or comments) after every multi-line function signature.
      # Default: false
      multi-func: true

    thelper:
      test:
        # Check *testing.T is first param (or after context.Context) of helper function.
        # Default: true
        first: true
        # Check *testing.T param has name t.
        # Default: true
        name: true
        # Check t.Helper() begins helper function.
        # Default: true
        begin: true
      benchmark:
        # Check *testing.B is first param (or after context.Context) of helper function.
        # Default: true
        first: true
        # Check *testing.B param has name b.
        # Default: true
        name: true
        # Check b.Helper() begins helper function.
        # Default: true
        begin: true
      tb:
        # Check *testing.TB is first param (or after context.Context) of helper function.
        # Default: true
        first: true
        # Check *testing.TB param has name tb.
        # Default: true
        name: true
        # Check tb.Helper() begins helper function.
        # Default: true
        begin: true

    testifylint:
      # Enable all checkers (https://github.com/Antonboom/testifylint#checkers).
      # Default: false
      enable-all: true

    nestif:
      # Minimal complexity of if statements to report.
      # Default: 5
      min-complexity: 5

    misspell:
      # Correct spellings using locale preferences for US or UK.
      # Setting locale to US will correct the British spelling of 'colour' to 'color'.
      # Default is to use a neutral variety of English.
      locale: US
      # Typos to ignore.
      # Should be in lower case.
      # Default: []

      extra-words:
        - typo: 'iff'
          correction: 'if'
        - typo: 'cancelation'
          correction: 'cancellation'
      # Mode of the analysis:
      # - default: checks all the file content.
      # - restricted: checks only comments.
      # Default: ""
      mode: default
    makezero:
      # Allow only slices initialized with a length of zero.
      # Default: false
      always: true
    maintidx:
      # Show functions with maintainability index lower than N.
      # A high index indicates better maintainability (it's kind of the opposite of complexity).
      # Default: 20
      under: 20

    govet:
      enable-all: true
    godox:
      # Report any comments starting with keywords, this is useful for TODO or FIXME comments that
      # might be left in the code accidentally and should be resolved before merging.
      # Default: ["TODO", "BUG", "FIXME"]
      keywords:
        - NOTE
        - OPTIMIZE # marks code that should be optimized before merging
        - HACK # marks hack-around that should be removed before merging
        - TODO
        - PLACEHOLDER
        - FIXME
        - BUG
    copyloopvar:
      # Check all assigning the loop variable to another variable.
      # Default: false
      check-alias: true
    exhaustive:
      # Program elements to check for exhaustiveness.
      # Default: [ switch ]
      check:
        - switch
        - map

    # Complexity, duplication, function size
    cyclop:
      max-complexity: 10
      package-average: 5
    gocognit:
      min-complexity: 5
    gocyclo:
      min-complexity: 5
    funlen:
      lines: 60
      statements: 40
      ignore-comments: false

    # Style, correctness, performance (opinionated but aligned with standards)
    gocritic:
      enabled-tags: [diagnostic, style, performance]

    # Disallow problematic imports/usages to keep security and consistency
    depguard:
      rules:
        main:
          list-mode: lax
          files: ['$all']
          allow:
            - $gostd
          deny:
            - pkg: 'github.com/sirupsen/logrus'
              desc: not allowed
            - pkg: 'github.com/pkg/errors'
              desc: Should be replaced by standard lib errors package
            - pkg: 'math/rand$'
              desc: use math/rand/v2
    forbidigo:
      forbid:
        - pattern: '^print(ln)?$'
        - pattern: "^fmt\\.Print.*$"
          msg: Do not commit print statements.
      analyze-types: true
      exclude-godoc-examples: true
    gosec:
      # To select a subset of rules to run.
      # Available rules: https://github.com/securego/gosec#available-rules
      # Default: [] - means include all rules
      includes:
        - G101 # Look for hard coded credentials
        - G102 # Bind to all interfaces
        - G103 # Audit the use of unsafe block
        - G104 # Audit errors not checked
        - G106 # Audit the use of ssh.InsecureIgnoreHostKey
        - G107 # Url provided to HTTP request as taint input
        - G108 # Profiling endpoint automatically exposed on /debug/pprof
        - G109 # Potential Integer overflow made by strconv.Atoi result conversion to int16/32
        - G110 # Potential DoS vulnerability via decompression bomb
        - G111 # Potential directory traversal
        - G112 # Potential slowloris attack
        - G114 # Use of net/http serve function that has no support for setting timeouts
        - G115 # Potential integer overflow when converting between integer types
        - G201 # SQL query construction using format string
        - G202 # SQL query construction using string concatenation
        - G203 # Use of unescaped data in HTML templates
        - G301 # Poor file permissions used when creating a directory
        - G302 # Poor file permissions used with chmod
        - G303 # Creating tempfile using a predictable path
        - G305 # File traversal when extracting zip/tar archive
        - G306 # Poor file permissions used when writing to a new file
        - G307 # Poor file permissions used when creating a file with os.Create
        - G401 # Detect the usage of MD5 or SHA1
        - G402 # Look for bad TLS connection settings
        - G403 # Ensure minimum RSA key length of 2048 bits
        - G404 # Insecure random number source (rand)
        - G405 # Detect the usage of DES or RC4
        - G406 # Detect the usage of MD4 or RIPEMD160
        - G501 # Import blocklist: crypto/md5
        - G502 # Import blocklist: crypto/des
        - G503 # Import blocklist: crypto/rc4
        - G504 # Import blocklist: net/http/cgi
        - G505 # Import blocklist: crypto/sha1
        - G506 # Import blocklist: golang.org/x/crypto/md4
        - G507 # Import blocklist: golang.org/x/crypto/ripemd160
        - G601 # Implicit memory aliasing of items from a range statement
        - G602 # Slice access out of bounds
        - G101 # Look for hard coded credentials
        - G102 # Bind to all interfaces
        - G103 # Audit the use of unsafe block
        - G104 # Audit errors not checked
        - G106 # Audit the use of ssh.InsecureIgnoreHostKey
        - G107 # Url provided to HTTP request as taint input
        - G108 # Profiling endpoint automatically exposed on /debug/pprof
        - G109 # Potential Integer overflow made by strconv.Atoi result conversion to int16/32
        - G110 # Potential DoS vulnerability via decompression bomb
        - G111 # Potential directory traversal
        - G112 # Potential slowloris attack
        - G114 # Use of net/http serve function that has no support for setting timeouts
        - G115 # Potential integer overflow when converting between integer types
        - G201 # SQL query construction using format string
        - G202 # SQL query construction using string concatenation
        - G203 # Use of unescaped data in HTML templates
        - G301 # Poor file permissions used when creating a directory
        - G302 # Poor file permissions used with chmod
        - G303 # Creating tempfile using a predictable path
        - G305 # File traversal when extracting zip/tar archive
        - G306 # Poor file permissions used when writing to a new file
        - G307 # Poor file permissions used when creating a file with os.Create
        - G401 # Detect the usage of MD5 or SHA1
        - G402 # Look for bad TLS connection settings
        - G403 # Ensure minimum RSA key length of 2048 bits
        - G404 # Insecure random number source (rand)
        - G405 # Detect the usage of DES or RC4
        - G406 # Detect the usage of MD4 or RIPEMD160
        - G501 # Import blocklist: crypto/md5
        - G502 # Import blocklist: crypto/des
        - G503 # Import blocklist: crypto/rc4
        - G504 # Import blocklist: net/http/cgi
        - G505 # Import blocklist: crypto/sha1
        - G506 # Import blocklist: golang.org/x/crypto/md4
        - G507 # Import blocklist: golang.org/x/crypto/ripemd160
        - G601 # Implicit memory aliasing of items from a range statement
        - G602 # Slice access out of bounds
      # Filter out the issues with a lower severity than the given value.
      # Valid options are: low, medium, high.
      # Default: low
      severity: low
      # Filter out the issues with a lower confidence than the given value.
      # Valid options are: low, medium, high.
      # Default: low
      confidence: medium
      # Concurrency value.
      # Default: the number of logical CPUs usable by the current process.
      concurrency: 12
      # To specify the configuration of rules.
      config:
        # Globals are applicable to all rules.
        global:
          # If true, ignore #nosec in comments (and an alternative as well).
          # Default: false
          nosec: true
          # Add an alternative comment prefix to #nosec (both will work at the same time).
          # Default: ""
          '#nosec': '#my-custom-nosec'
          # Define whether nosec issues are counted as finding or not.
          # Default: false
          show-ignored: true
          # Audit mode enables addition checks that for normal code analysis might be too nosy.
          # Default: false
          audit: true
        G101:
          # Regexp pattern for variables and constants to find.
          # Default: "(?i)passwd|pass|password|pwd|secret|token|pw|apiKey|bearer|cred"
          pattern: '(?i)example'
          # If true, complain about all cases (even with low entropy).
          # Default: false
          ignore_entropy: false
          # Maximum allowed entropy of the string.
          # Default: "80.0"
          entropy_threshold: '80.0'
          # Maximum allowed value of entropy/string length.
          # Is taken into account if entropy >= entropy_threshold/2.
          # Default: "3.0"
          per_char_threshold: '3.0'
          # Calculate entropy for first N chars of the string.
          # Default: "16"
          truncate: '32'
        G104:
          fmt:
            - Fscanf
        G111:
          # Regexp pattern to find potential directory traversal.
          # Default: "http\\.Dir\\(\"\\/\"\\)|http\\.Dir\\('\\/'\\)"
          pattern: "custom\\.Dir\\(\\)"
        # Maximum allowed permissions mode for os.Mkdir and os.MkdirAll.
        # Default: "0750"
        G301: '0750'
        # Maximum allowed permissions mode for os.OpenFile and os.Chmod.
        # Default: "0600"
        G302: '0600'
        # Maximum allowed permissions mode for os.WriteFile and ioutil.WriteFile.
        # Default: "0600"
        G306: '0600'

formatters:
  enable:
    - gci
    - gofmt
    - gofumpt
    - goimports
    - golines
    - swaggo # only available since v2.2.0; remove if on an older version

  exclusions:
    warn-unused: true
    generated: strict
    paths:
      - ".*\\.my\\.go$"
      - 'lib/bad.go'

  settings:
    # Import grouping: stdlib -> third-party -> local module
    gci:
      custom-order: true
      sections:
        - standard
        - default
        - localmodule
      no-inline-comments: false
      no-prefix-comments: false
      no-lex-order: false

    # Canonical gofmt with optional rewrite rules
    gofmt:
      simplify: true
      rewrite-rules:
        - pattern: 'interface{}'
          replacement: 'any'
        - pattern: 'a[b:len(a)]'
          replacement: 'a[b:]'

    # Modern formatting refinements
    gofumpt:
      extra-rules: true

    # Line-wrapping for readability
    golines:
      max-len: 90
      tab-len: 8
      shorten-comments: true
      reformat-tags: true
      chain-split-dots: true

    # Note: swaggo has no configurable settings
# Move fix to the run section (not under issues)
# Output-related toggles (uniq-by-line belongs here, not in issues)
output:
  # The formats used to render issues.
  formats:
    # Prints issues in a text format with colors, line number, and linter name.
    # This format is the default format.
    text:
      # Output path can be either `stdout`, `stderr` or path to the file to write to.
      # Default: stdout
      path: stdout
      # Print linter name in the end of issue text.
      # Default: true
      print-linter-name: true
      # Print lines of code with issue.
      # Default: true
      print-issued-lines: true
      # Use colors.
      # Default: true
      colors: true

  show-stats: true

# Issue filtering and new-code gating
issues:
  whole-files: true
  fix: true
