# Product screenshots

The images in `docs/assets/screenshots/` come from the real desktop UI. The capture script loads `internal/desktop/assets/index.html` and `app.js`, and replaces only the Go backend with fixture data. Regenerate the images after any change to the UI, so the README never shows an old screen.

## Regenerate

From the repository root:

```powershell
node scripts/screenshots/capture.cjs
```

To render one shot:

```powershell
node scripts/screenshots/capture.cjs --only compact-running
```

To check that every scenario still runs without writing into `docs/`:

```powershell
node scripts/screenshots/capture.cjs --check
```

Each shot is written only after its scenario finishes. If a step throws, or the page does not finish within the capture budget, the script stops and prints the page error and the browser console.

Images are rendered at 2× device pixels, so they stay sharp on high-DPI screens. The layout is the same as the app's. Set `TESTDESK_SHOT_SCALE=1` to produce smaller files.

## Requirements

- Node.js 18 or later. The script has no npm dependencies.
- A Chromium-based browser. The script looks for the newest `chromium_headless_shell-*` in `%LOCALAPPDATA%\ms-playwright` (installed by Playwright), then Microsoft Edge. Set `TESTDESK_BROWSER_PATH` to use another binary.

Microsoft Edge's GUI build returned before writing output on the machine where this was set up. Use the headless shell if Edge produces no image.

## What the script does

- `scripts/screenshots/scenarios.cjs` holds the sample data and the list of shots. Each shot names a screen, a window size, a fixture snapshot, and the steps that reach the screen (for example `setMode('compact')`).
- `scripts/screenshots/capture.cjs` builds one HTML file per shot. It injects a fixture bridge for `window.go.desktop.App`, runs the shot's steps after the app boots, and renders the page at the shot's size.

Sample data uses a neutral project root (`C:\Users\demo\Documents\GitHub\...`). Test names are real names from the TestDesk and Workcopy Go suites, but the statuses, durations and the one failure are sample values.

## Adding or changing a shot

1. Add an entry to `SHOTS` in `scenarios.cjs`, with a unique `name` and a `description`. Keep one screen per shot, and do not repeat a screen that another shot already shows.
2. If the screen needs data that the fixtures lack, add it to `buildFixtures`.
3. Run `node scripts/screenshots/capture.cjs --only <name>` and open the PNG to check it.
4. Add the image to the README or the relevant doc.

## Known limits

- The fixture bridge only covers the calls the screens make. A call it does not know returns `undefined`, which may show up as an empty area in the image.
- The shots show the UI in a fixed state. They do not capture a live run, hover states or animations.
- Edge and the headless shell can differ in font rendering. Regenerate every shot with the same browser so the images match.
