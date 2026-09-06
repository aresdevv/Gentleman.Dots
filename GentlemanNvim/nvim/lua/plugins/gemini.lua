return {
  "jonroosevelt/gemini-cli.nvim",
  -- Lazy-load on the plugin's own keymaps instead of at startup. The
  -- plugin's `plugin/gemini.vim` already calls `require("gemini").setup()`
  -- itself as soon as the plugin loads, so there is no need (and no safe
  -- way) to also call setup() from `config` here: doing so ran setup()
  -- twice on every startup, and since setup() falls back to a blocking
  -- `vim.fn.input(...)` prompt when the `gemini` CLI isn't on $PATH, that
  -- doubled prompt could block Neovim startup entirely (and hang forever
  -- under `nvim --headless`). Deferring the load to these keys means
  -- setup() only ever runs once, on demand, when the plugin manager
  -- actually loads the plugin. See issue #173.
  keys = {
    { "<leader>og", desc = "Toggle Gemini CLI" },
    { "<leader>sg", mode = "v", desc = "Send selection to Gemini" },
  },
}
