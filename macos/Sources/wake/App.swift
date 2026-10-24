import ServiceManagement
import SwiftUI
import Vero
import VeroUpdates

// wake for macOS: an item in the menu bar, whose panel turns keeping
// the Mac awake after its lid is closed on and off, on the worker in
// ../cmd/worker. WorkerAPI.swift beside this is what vero gen writes from
// the worker: Status, and a type for each request.

@main
struct WakeApp: App {
    init() {
        // One copy at a time: a second one hands over to the first and quits.
        VeroSingleCopy.yieldToRunningCopy()
    }

    // The worker, started from beside this app, or from the bundle's
    // Resources in an app bundle, and the last state it pushed.
    @StateObject private var vero = VeroModel<Status>(bundledWorker: "wake-worker", directoryName: "wake/bin")

    // Sparkle, through vero's VeroUpdater, kept for the life of the app.
    // vero's release puts the appcast's address and the public key from
    // vero-app.toml in Info.plist, so there is nothing to set here. A copy
    // run with VERO_PROFILE set does not check for updates at all.
    @StateObject private var updates = VeroUpdater()

    // Whether macOS opens the app at login.
    @StateObject private var loginItem = LoginItem()

    var body: some Scene {
        MenuBarExtra {
            Panel().environmentObject(vero).environmentObject(updates).environmentObject(loginItem)
        } label: {
            Image(systemName: vero.state?.on == true ? "cup.and.saucer.fill" : "cup.and.saucer")
        }
        .menuBarExtraStyle(.window)
    }
}

/// The panel under the menu bar item: the switch, what is going on in a
/// sentence, and the buttons that check for updates and quit.
struct Panel: View {
    @EnvironmentObject var vero: VeroModel<Status>
    @EnvironmentObject var updates: VeroUpdater
    @EnvironmentObject var loginItem: LoginItem

    private var status: Status { vero.state ?? Status() }

    /// What the worker says about the last attempt, or what the last
    /// request was refused with.
    private var problem: String? {
        if !status.problem.isEmpty { return status.problem }
        return vero.problem
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Toggle(isOn: Binding(
                get: { status.on },
                set: { on in
                    if on { vero.call(TurnOnRequest()) } else { vero.call(TurnOffRequest()) }
                }
            )) {
                Text("Keep awake when the lid is closed").font(.headline)
            }
            .toggleStyle(.switch)
            .disabled(vero.state == nil || !status.hasLid || status.phase == "stopping")

            Text(describe(status))
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

            if let problem {
                Text(problem)
                    .foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Divider()

            Toggle("Open at login", isOn: Binding(get: { loginItem.enabled }, set: { loginItem.set($0) }))
                .toggleStyle(.checkbox)
                .onAppear { loginItem.refresh() }
            if let problem = loginItem.problem {
                Text(problem)
                    .foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Divider()

            HStack {
                Button("Check for Updates…") { updates.checkForUpdates() }
                    .disabled(!updates.canCheckForUpdates)
                Spacer()
                Button("Quit") { NSApplication.shared.terminate(nil) }
                    .keyboardShortcut("q")
            }
        }
        .padding(16)
        .frame(width: 340)
    }

    /// The phase, in words.
    private func describe(_ s: Status) -> String {
        switch s.phase {
        case "nolid":
            return "This Mac has no lid."
        case "starting":
            return "Waiting for an administrator's password. Turning this on asks for one each time."
        case "open":
            return "Sleep is off until the lid closes, and the Mac stays awake once it is closed. On since \(s.since). A MacBook that is awake in a bag can get hot."
        case "closed":
            return "The lid is closed and the Mac is awake. Turning this off puts the Mac to sleep."
        case "stopping":
            return "Turning off."
        case "off":
            return "The Mac goes to sleep when its lid is closed, as usual."
        default:
            return "Starting the worker."
        }
    }
}

/// Whether macOS opens the app at login, asked of the system itself, which
/// shows the app under Login Items in System Settings. A copy that is not
/// an app bundle, such as the debug build, cannot be registered, and the
/// system's reason is shown.
@MainActor
final class LoginItem: ObservableObject {
    @Published private(set) var enabled = SMAppService.mainApp.status == .enabled
    @Published private(set) var problem: String?

    func refresh() {
        enabled = SMAppService.mainApp.status == .enabled
    }

    func set(_ on: Bool) {
        do {
            if on { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
            problem = nil
        } catch {
            problem = "macOS did not allow this. \(error.localizedDescription)"
        }
        refresh()
    }
}
