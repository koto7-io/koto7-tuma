# Tuma brand tokens

Canonical source: `koto7-tuma/web/src/styles/tokens.css`

Marketing site (`koto7-web/tuma/assets/tokens.css`) and Tailwind config in `index.html` mirror these values — keep in sync when changing tokens.

## Themes

- **Default:** dark (mesh cyan `#38FFF6` accent) — matches marketing site
- **Light:** mesh-adapted teal accent `#028578` on warm surfaces; mesh palette unchanged for SVG assets
- **Toggle:** Light → Dark → System (stored in `localStorage` key `tuma-theme`)

UI tokens use `--accent` / `--on-accent` for interactive elements. Raw mesh routes (`--mesh-a/b/c`) are for SVG strokes and glow effects.

## Icon tiers

| Size | Asset | Use |
|---|---|---|
| ≤48px | `tuma-mark-nav.svg` | Nav, favicon contexts, flow strip center |
| 96px | `tuma-badger-loading.svg` | Loading states, flow strip during retries |
| ≥480px | `tuma-badger-hero.svg` / `tuma-badger-static.svg` | Hero, storytelling |

## Typography

- **Body:** Inter
- **Brand wordmark:** IBM Plex Mono (`tuma`, nav lockups)
- **Code / logs:** JetBrains Mono

## Mesh signals

- Route A: `#38FFF6` (`--mesh-a`)
- Route B: `#20E2D7` (`--mesh-b`)
- Route C: `#02C39A` (`--mesh-c`)

Stroke palette: `--mesh-stroke-1` … `--mesh-stroke-8` (see tokens.css).

## Playground flow strip states

| State | Provider | Tuma mark | Destination |
|---|---|---|---|
| Healthy | active | nav mark + glow | active |
| Broken | active | nav mark | failing (red) |
| Retrying | active | loading badger (Route A) | failing |
| Issue held | active | holding (amber) | failing or idle |
