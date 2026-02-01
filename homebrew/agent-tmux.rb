# Homebrew alias formula for atmux (agent-tmux)
# To use locally: brew install --build-from-source ./homebrew/agent-tmux.rb

class AgentTmux < Formula
  desc "Alias for atmux (agent-tmux)"
  homepage "https://github.com/organisciak/atmux"
  url "https://github.com/organisciak/atmux/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "REPLACE_WITH_ACTUAL_SHA256"
  license "MIT"
  head "https://github.com/organisciak/atmux.git", branch: "main"

  depends_on "atmux"

  def install
    bin.install_symlink Formula["atmux"].opt_bin/"atmux" => "agent-tmux"
  end

  test do
    assert_match "atmux", shell_output("#{bin}/agent-tmux --help")
  end
end
