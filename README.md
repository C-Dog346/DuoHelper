DISCLAIMER: I make no guarantees of credential safety. Use at your own risk. 

# DuoHelper

DuoHelper alerts you (on Windows) when your daily Duolingo lesson hasn't been completed by a configured time. It uses a Duolingo JWT token to query the (undocumented) Duolingo API and determine whether your streak/lesson for the day has been completed.

Status: Work in progress — currently tested locally on Windows.

## Features

- Check whether the day's lesson/streak has been extended using a Duolingo JWT token
- Send a local notification (planned)
- Intended to be run from Task Scheduler or a similar scheduler

## Requirements

- Go 1.20+ (see `go.mod`)
- Windows (for native notification integration; core check should be cross-platform)
- A `tokens.json` file containing your Duolingo JWT token (see example below)

## tokens.json

Create a `tokens.json` file in the project directory with the following structure:

{
"jwt_token": "your_duolingo_jwt_here"
}

Keep this file private — it contains authentication credentials.

## Build & Run

To build the binary locally:

```bash
go build -o bin/duohelper ./cmd
```

Run directly (development):

```bash
go run ./cmd
```

## Running in production / Scheduling

Use Windows Task Scheduler to run the built binary at the desired time each day. Configure the task to run `bin/duohelper` and ensure the working directory contains `tokens.json`.

## TODO

- Send Windows notification when a lesson is missing
- Add automated token refresh flow or an easier re-auth flow when the token expires
- Add tests and CI
- Improve cross-platform support

## Security

- Do not commit `tokens.json` or any secrets to version control. Add it to `.gitignore` if you store it in the repo root.

## License

MIT — see LICENSE file if present.

If you'd like, I can also add a small example `tokens.json` template, update `.gitignore`, or wire up a basic Windows notification. Which would you prefer next?
