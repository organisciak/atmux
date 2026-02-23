# Homebrew alias formula for atmux (agent-tmux)
# To use locally: brew install --build-from-source ./homebrew/agent-tmux.rb

class AgentTmux < Formula
  desc "Alias for atmux (agent-tmux)"
  homepage "https://github.com/organisciak/atmux"
  url "https://github.com/organisciak/atmux/archive/refs/tags/v0.2.0.tar.gz"
  sha256 "1daef7cfa2d46fbdbcdabe3322ba8ac43ca0b3dcd2b13df7240de27c10e77f7d"
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
