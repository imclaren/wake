// Written by vero gen from the worker's own types. Do not edit it: change
// the worker, then run vero gen.

import Foundation
import Vero

/// Status is the state: everything the menu shows.
struct Status: Codable, Hashable, Sendable {
    /// On is whether it has been turned on at the menu bar.
    var on: Bool = false
    /// Phase is one of the phases above.
    var phase: String = ""
    /// Mode is "open", to keep the Mac awake while its lid is open, or
    /// "closed", to keep it awake even after the lid is closed.
    var mode: String = ""
    /// Display is whether the display is kept on as well.
    var display: Bool = false
    /// Minutes is how long it stays on once turned on, and 0 is until it is
    /// turned off.
    var minutes: Int = 0
    /// Until is when the timer turns it off, as HH:MM, and empty without one.
    var until: String = ""
    /// LidClosed is whether the lid is closed now.
    var lidClosed: Bool = false
    /// SleepDisabled is the system's own SleepDisabled setting.
    var sleepDisabled: Bool = false
    /// HasLid is whether this Mac has a lid at all.
    var hasLid: Bool = false
    /// RuleInstalled is whether the rule for sudo is in place, so that the
    /// closed mode can be turned on without asking for a password.
    var ruleInstalled: Bool = false
    /// Since is when it was turned on, as HH:MM.
    var since: String = ""
    /// Problem is why the last attempt to turn it on failed, in a sentence.
    var problem: String = ""
    /// Remember is whether the switch is remembered across logins: turned
    /// on again when the worker starts, if it was on when it last ran.
    var remember: Bool = false

    enum CodingKeys: String, CodingKey {
        case on
        case phase
        case mode
        case display
        case minutes
        case until
        case lidClosed
        case sleepDisabled
        case hasLid
        case ruleInstalled
        case since
        case problem
        case remember
    }
}

extension Status {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        on = try c.decodeIfPresent(Bool.self, forKey: .on) ?? false
        phase = try c.decodeIfPresent(String.self, forKey: .phase) ?? ""
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? ""
        display = try c.decodeIfPresent(Bool.self, forKey: .display) ?? false
        minutes = try c.decodeIfPresent(Int.self, forKey: .minutes) ?? 0
        until = try c.decodeIfPresent(String.self, forKey: .until) ?? ""
        lidClosed = try c.decodeIfPresent(Bool.self, forKey: .lidClosed) ?? false
        sleepDisabled = try c.decodeIfPresent(Bool.self, forKey: .sleepDisabled) ?? false
        hasLid = try c.decodeIfPresent(Bool.self, forKey: .hasLid) ?? false
        ruleInstalled = try c.decodeIfPresent(Bool.self, forKey: .ruleInstalled) ?? false
        since = try c.decodeIfPresent(String.self, forKey: .since) ?? ""
        problem = try c.decodeIfPresent(String.self, forKey: .problem) ?? ""
        remember = try c.decodeIfPresent(Bool.self, forKey: .remember) ?? false
    }
    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(on, forKey: .on)
        try c.encode(phase, forKey: .phase)
        try c.encode(mode, forKey: .mode)
        try c.encode(display, forKey: .display)
        try c.encode(minutes, forKey: .minutes)
        try c.encode(until, forKey: .until)
        try c.encode(lidClosed, forKey: .lidClosed)
        try c.encode(sleepDisabled, forKey: .sleepDisabled)
        try c.encode(hasLid, forKey: .hasLid)
        try c.encode(ruleInstalled, forKey: .ruleInstalled)
        try c.encode(since, forKey: .since)
        try c.encode(problem, forKey: .problem)
        try c.encode(remember, forKey: .remember)
    }
}

struct RememberRequest: NamedRequest {
    typealias Reply = Status
    static let name = "remember"
    /// On is whether to remember the switch across logins.
    var on: Bool = false

    enum CodingKeys: String, CodingKey {
        case on
    }
}

struct SetOptionsRequest: NamedRequest {
    typealias Reply = Status
    static let name = "setOptions"
    /// Mode is "open" or "closed".
    var mode: String = ""
    /// Display is whether to keep the display on as well.
    var display: Bool = false
    /// Minutes is how long to stay on, and 0 is until it is turned off.
    var minutes: Int = 0

    enum CodingKeys: String, CodingKey {
        case mode
        case display
        case minutes
    }
}

/// Update handles a request with no accompanying data. "status" returns
/// the state, "turnOn" keeps the Mac awake in the chosen mode, and
/// "turnOff" lets it sleep again. "setOptions" sets the mode, the display
/// and the timer, and takes an Options. "remember" sets whether the switch
/// is remembered across logins, and takes a Remember.
struct StatusRequest: NamedRequest {
    typealias Reply = Status
    static let name = "status"
}

struct TurnOffRequest: NamedRequest {
    typealias Reply = Status
    static let name = "turnOff"
}

struct TurnOnRequest: NamedRequest {
    typealias Reply = Status
    static let name = "turnOn"
}
