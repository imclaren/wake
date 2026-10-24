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
    /// LidClosed is whether the lid is closed now.
    var lidClosed: Bool = false
    /// SleepDisabled is the system's own SleepDisabled setting.
    var sleepDisabled: Bool = false
    /// HasLid is whether this Mac has a lid at all.
    var hasLid: Bool = false
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
        case lidClosed
        case sleepDisabled
        case hasLid
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
        lidClosed = try c.decodeIfPresent(Bool.self, forKey: .lidClosed) ?? false
        sleepDisabled = try c.decodeIfPresent(Bool.self, forKey: .sleepDisabled) ?? false
        hasLid = try c.decodeIfPresent(Bool.self, forKey: .hasLid) ?? false
        since = try c.decodeIfPresent(String.self, forKey: .since) ?? ""
        problem = try c.decodeIfPresent(String.self, forKey: .problem) ?? ""
        remember = try c.decodeIfPresent(Bool.self, forKey: .remember) ?? false
    }
    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(on, forKey: .on)
        try c.encode(phase, forKey: .phase)
        try c.encode(lidClosed, forKey: .lidClosed)
        try c.encode(sleepDisabled, forKey: .sleepDisabled)
        try c.encode(hasLid, forKey: .hasLid)
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

/// Update handles a request with no accompanying data. "status" returns
/// the state, "turnOn" keeps the Mac awake after its lid is closed, and
/// "turnOff" lets it sleep again. "remember" sets whether the switch is
/// remembered across logins, and takes a Remember.
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
