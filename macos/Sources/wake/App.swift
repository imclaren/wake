import ServiceManagement
import SwiftUI
import Vero
import VeroUpdates

// wake for macOS: an item in the menu bar, whose panel keeps the Mac
// awake, while its lid is open or even after it is closed, on the worker in
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

/// The panel under the menu bar item: the switch, the mode, the display and
/// the timer, what is going on in a sentence, and the buttons that check for
/// updates and quit.
struct Panel: View {
    @EnvironmentObject var vero: VeroModel<Status>
    @EnvironmentObject var updates: VeroUpdater
    @EnvironmentObject var loginItem: LoginItem

    /// An action that would ask for the administrator's password, held back
    /// until the person has read why and chosen to go on.
    enum PasswordAction { case turnOn, switchToClosed }
    @State private var pending: PasswordAction?

    private var status: Status { vero.state ?? Status() }

    /// What the worker says about the last attempt, or what the last
    /// request was refused with.
    private var problem: String? {
        if !status.problem.isEmpty { return status.problem }
        return vero.problem
    }

    /// The timer's choices, in minutes, and 0 is until it is turned off.
    private let durations: [(minutes: Int, label: String)] = [
        (0, "Never"), (60, "After 1 hour"), (120, "After 2 hours"), (240, "After 4 hours"), (480, "After 8 hours"),
    ]

    private func setOptions(mode: String? = nil, display: Bool? = nil, minutes: Int? = nil) {
        vero.call(SetOptionsRequest(
            mode: mode ?? status.mode,
            display: display ?? status.display,
            minutes: minutes ?? status.minutes))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Toggle(isOn: Binding(
                get: { status.on },
                set: { on in
                    if !on {
                        pending = nil
                        vero.call(TurnOffRequest())
                    } else if status.mode == "closed" && !status.ruleInstalled {
                        pending = .turnOn
                    } else {
                        vero.call(TurnOnRequest())
                    }
                }
            )) {
                Text("Keep this Mac awake").font(.headline)
            }
            .toggleStyle(.switch)
            .disabled(vero.state == nil || status.phase == "stopping" || pending != nil)

            Picker("When", selection: Binding(
                get: { status.mode },
                set: { mode in
                    if mode == "closed" && status.on && !status.ruleInstalled {
                        pending = .switchToClosed
                    } else {
                        setOptions(mode: mode)
                    }
                }
            )) {
                Text("While the lid is open").tag("open")
                Text("Even when the lid is closed").tag("closed")
            }
            .pickerStyle(.radioGroup)
            .labelsHidden()
            .disabled(vero.state == nil || !status.hasLid || status.phase == "starting" || pending != nil)

            if let pending {
                PasswordExplanation(
                    proceed: {
                        self.pending = nil
                        switch pending {
                        case .turnOn: vero.call(TurnOnRequest())
                        case .switchToClosed: setOptions(mode: "closed")
                        }
                    },
                    cancel: { self.pending = nil })
            }

            Toggle("Keep display on while wake is on", isOn: Binding(
                get: { status.display },
                set: { setOptions(display: $0) }))
                .toggleStyle(.checkbox)
                .disabled(vero.state == nil)

            Picker("Turn wake off", selection: Binding(
                get: { status.minutes },
                set: { setOptions(minutes: $0) }
            )) {
                ForEach(durations, id: \.minutes) { Text($0.label).tag($0.minutes) }
            }
            .fixedSize()
            .disabled(vero.state == nil)

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

            Toggle("Remember across logins", isOn: Binding(
                get: { status.remember },
                set: { on in vero.call(RememberRequest(on: on)) }
            ))
            .toggleStyle(.checkbox)
            .disabled(vero.state == nil)
            Text("Leaving this and the switch on means the Mac never sleeps, since wake turns the switch on again at every login.")
                .font(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

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

    /// How long it stays on, as a sentence.
    private func howLong(_ s: Status) -> String {
        s.until.isEmpty ? "On since \(s.since)." : "On until \(s.until)."
    }

    /// The phase, in words.
    private func describe(_ s: Status) -> String {
        let display = s.display ? "The display stays on." : "The display turns off as usual."
        switch s.phase {
        case "nolid":
            return "This Mac has no lid. Turn the switch on to keep it awake."
        case "awake":
            if !s.hasLid {
                return "The Mac stays awake. \(display) \(howLong(s))"
            }
            return "The Mac stays awake while the lid is open, and sleeps as usual when it is closed. \(display) \(howLong(s))"
        case "starting":
            return "Waiting for an administrator's password. It is asked for once, the first time the Mac is kept awake with its lid closed."
        case "open":
            return "Sleep is off until the lid closes, and the Mac stays awake once it is closed. \(howLong(s)) A MacBook that is awake in a bag can get hot."
        case "closed":
            return "The lid is closed and the Mac is awake. Turning this off puts the Mac to sleep. \(howLong(s))"
        case "stopping":
            return "Turning off."
        case "off":
            if s.mode == "closed" {
                return "The Mac sleeps as usual. Turned on, it stays awake even after the lid is closed."
            }
            return "The Mac sleeps as usual. Turned on, it stays awake while the lid is open, and sleeps when the lid is closed."
        default:
            return "Starting the worker."
        }
    }
}

/// Why the administrator's password is about to be asked for, shown before
/// the system's own dialog, with the choice to go on or not.
struct PasswordExplanation: View {
    let proceed: () -> Void
    let cancel: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("An administrator's password is needed once").font(.subheadline.weight(.semibold))
            Text("A Mac goes to sleep when its lid closes, and only an administrator may change that. When you continue, macOS asks for an administrator's password. wake then adds a rule that lets this account turn that one setting on and off, and put the Mac to sleep, without the password. It allows nothing else, and the password is not asked for again.")
                .font(.callout)
                .fixedSize(horizontal: false, vertical: true)
            Text("The rule is in /private/etc/sudoers.d, and removing its file undoes it.")
                .font(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack {
                Spacer()
                Button("Cancel", action: cancel)
                Button("Continue", action: proceed)
                    .keyboardShortcut(.defaultAction)
            }
        }
        .padding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.secondary.opacity(0.12)))
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
