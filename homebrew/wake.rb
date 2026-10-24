cask "wake" do
  version "0.2.0"
  sha256 "9f40c23712ae7c3cadd569ceb70c64f369d9b2611aca78ff008aa9f2beea523a"

  url "https://wake.calmapps.com/releases/macos/wake-#{version}-macos.dmg"
  name "wake"
  desc "Keeps a Mac awake, even with its lid closed"
  homepage "https://wake.calmapps.com"

  app "wake.app"
end
