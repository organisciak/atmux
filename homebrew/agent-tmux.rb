# Homebrew formula for agent-tmux
# To use locally: brew install --build-from-source ./homebrew/agent-tmux.rb

class AgentTmux < Formula
  desc "Manage tmux sessions for AI coding agents"
  homepage "https://github.com/porganisciak/agent-tmux"
  url "https://github.com/porganisciak/agent-tmux/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "REPLACE_WITH_ACTUAL_SHA256"
  license "MIT"
  head "https://github.com/porganisciak/agent-tmux.git", branch: "main"

  depends_on "go" => :build
  depends_on "tmux"

  def install
    ldflags = %W[
      -s -w
      -X github.com/porganisciak/agent-tmux/cmd.Version=#{version}
      -X github.com/porganisciak/agent-tmux/cmd.Commit=#{tap.user}
      -X github.com/porganisciak/agent-tmux/cmd.BuildDate=#{time.iso8601}
    ]
    system "go", "build", *std_go_args(ldflags:)

    generate_completions_from_executable(bin/"agent-tmux", "completion")
  end

  test do
    assert_match "agent-tmux", shell_output("#{bin}/agent-tmux --help")
    assert_match version.to_s, shell_output("#{bin}/agent-tmux version")
  end
end
