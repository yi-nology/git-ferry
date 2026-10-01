# Homebrew formula 模板（发布到 yi-nology/homebrew-tap 时按版本改 URL/sha256）
class GitFerry < Formula
  desc "Self-hosted Git sync / mirror / backup hub"
  homepage "https://github.com/yi-nology/git-ferry"
  version "1.19.1"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/yi-nology/git-ferry/releases/download/v#{version}/git-ferry_darwin_arm64.tar.gz"
      sha256 "REPLACE_ME"
    end
    on_intel do
      url "https://github.com/yi-nology/git-ferry/releases/download/v#{version}/git-ferry_darwin_amd64.tar.gz"
      sha256 "REPLACE_ME"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/yi-nology/git-ferry/releases/download/v#{version}/git-ferry_linux_amd64.tar.gz"
      sha256 "REPLACE_ME"
    end
    on_arm do
      url "https://github.com/yi-nology/git-ferry/releases/download/v#{version}/git-ferry_linux_arm64.tar.gz"
      sha256 "REPLACE_ME"
    end
  end

  def install
    bin.install "git-ferry"
    bin.install "gitferry" if File.exist?("gitferry")
  end

  service do
    run [opt_bin/"git-ferry"]
    keep_alive true
    working_dir var/"gitferry"
  end

  test do
    assert_match "git-ferry", shell_output("#{bin}/git-ferry --help 2>&1", 1) rescue system "#{bin}/git-ferry", "--help"
  end
end
