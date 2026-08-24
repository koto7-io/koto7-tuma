# Push Tuma to GitHub (Desktop app)

Use this if your terminal git is tied to Bitbucket — GitHub Desktop keeps remotes separate.

## 1. Create empty repo on GitHub

In the browser: **github.com → New repository**

- Name: `tuma` (or `koto7-tuma`)
- Public (for OSS) or Private
- **Do not** add README, .gitignore, or license (this repo already has them)

Copy the repo URL, e.g. `https://github.com/koto7/tuma.git`

## 2. Add repo in GitHub Desktop

**If the folder is not a git repo yet:**

1. Open GitHub Desktop
2. **File → Add Local Repository**
3. Choose `/Users/ashwin/dev/repos/koto7/koto7-tuma`
4. If prompted “not a git repository” → **create a repository** here (init with name `koto7-tuma`)

**If already initialized:**

1. **File → Add Local Repository** → select the folder

## 3. First commit

In GitHub Desktop, review changed files. You should **not** see:

- `web/node_modules/`
- `web/dist/`
- `.env` files

Commit message suggestion:

```
Initial Tuma OSS v1 — webhook reliability layer
```

Click **Commit to main**.

## 4. Publish to GitHub (not Bitbucket)

1. **Repository → Repository settings** — confirm this repo’s remote is GitHub, not Bitbucket
2. **Publish repository** (first time) or **Push origin**
3. When asked for remote URL, paste the **GitHub** URL from step 1
4. Ensure you’re signed into the correct GitHub account/org in Desktop (**Preferences → Accounts**)

If Desktop tries to use Bitbucket, remove the Bitbucket remote:

- **Repository → Repository settings → Remote** — primary remote should be `origin` → `github.com/...`

## 5. Team handoff

Share:

- Repo URL
- README **Testing without Stripe** section
- `deploy/.env.example` for prod secrets

After team sign-off, create a release tag on GitHub: **Releases → Draft new release → v0.1.0**
