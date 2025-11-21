# PHP Import Sorter (psort)

A high-performance Go script to sort and organize PHP `use` statements (imports) in your project. It supports parallel processing, custom grouping, and configurable spacing.

## Features

- **Fast**: Written in Go, uses goroutines for parallel file processing.
- **Safe**: Uses atomic file writes to prevent data loss.
- **Flexible**: Configurable via `psort.json`.
- **Smart**: Handles empty lines, consolidates imports, and supports custom sort orders (e.g., Vendor first, then App).

## Installation

You can build the tool from source:

```bash
go build -o psort src/main.go
```

## Usage

### Single File Mode

To sort imports in a specific file:

```bash
./psort path/to/file.php
```

### Project Mode

To process your entire project based on configuration:

```bash
./psort
```

This requires a `psort.json` configuration file in the current directory.

## Configuration (`psort.json`)

Create a `psort.json` file in your project root to configure the behavior.

### Options

- **include**: Array of file patterns to process.
    - `*.php`: Matches files in the root directory only (strict).
    - `**/*.php`: Matches files recursively in all subdirectories.
    - `app/*.php`: Matches files in the `app` directory.
- **exclude**: Array of patterns to ignore.
    - `vendor`: Excludes the `vendor` directory and its contents.
- **groups**: Array of strings defining the sort order.
    - `App\\`: Matches imports starting with `App\`.
    - `*`: Wildcard matching any import not matched by other groups.
    - Imports are sorted by their group index first, then alphabetically.
- **newline_between_groups**: Boolean (`true`/`false`).
    - If `true`, adds an empty line between different import groups.

### Example Configuration

```json
{
  "include": [
    "**/*.php"
  ],
  "exclude": [
    "vendor",
    ".git",
    "storage"
  ],
  "groups": [
    "*",
    "App\\"
  ],
  "newline_between_groups": true
}
```

**Explanation of Example:**
1.  **Include**: Processes all `.php` files recursively.
2.  **Exclude**: Ignores `vendor`, `.git`, and `storage` directories.
3.  **Groups**:
    - `*`: Matches 3rd party libraries (e.g., `Illuminate\Support\Facades\DB`) and places them **first**.
    - `App\\`: Matches application code (e.g., `App\Models\User`) and places them **second**.
4.  **Spacing**: Adds a blank line between the vendor imports and the app imports.

## How it Works

1.  **Scans**: Reads the file line by line.
2.  **Identifies**: Detects blocks of `use` statements.
3.  **Buffers**: Collects imports and any interleaved empty lines.
4.  **Sorts**: Sorts the collected imports based on your `groups` configuration.
5.  **Writes**: Writes the sorted block back to a temporary file, preserving surrounding code.
6.  **Replaces**: Atomically replaces the original file with the sorted version.
