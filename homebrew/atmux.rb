# Homebrew formula for atmux (agent-tmux)
# To use locally: brew install --build-from-source ./homebrew/atmux.rb

class Atmux < Formula
  desc "atmux (agent-tmux): manage tmux sessions for AI coding agents"
  homepage "https://github.com/organisciak/atmux"
  url "https://github.com/organisciak/atmux/archive/refs/tags/v0.3.0.tar.gz"
  sha256 "2e30f558d4c39a5f144b67387b70ddf769729770675c3eb795e566144b928140"
  license "MIT"
  head "https://github.com/organisciak/atmux.git", branch: "main"

  depends_on "go" => :build
  depends_on "tmux"

  def install
    ldflags = %W[
      -s -w
      -X github.com/porganisciak/agent-tmux/cmd.Version=#{version}
      -X github.com/porganisciak/agent-tmux/cmd.Commit=#{tap.user}
      -X github.com/porganisciak/agent-tmux/cmd.BuildDate=#{time.iso8601}
    ]
    system "go", "build", *std_go_args(output: bin/"atmux", ldflags: ldflags)

    generate_completions_from_executable(bin/"atmux", "completion")
  end

  test do
    assert_match "atmux", shell_output("#{bin}/atmux --help")
    assert_match version.to_s, shell_output("#{bin}/atmux version")
  end
end
