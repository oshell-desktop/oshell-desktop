import Foundation
import OShellCore

// Synthetic credentials only. The harness never opens application storage.
let master = "interop-fixture-master-🔐"
let args = CommandLine.arguments
func checked(_ condition: Bool, _ message: String) throws { if !condition {throw NSError(domain:message,code:1)} }
func validate(_ cfg: Configuration) throws {
 try checked(cfg.profiles.count == 3, "profile count")
 try checked(cfg.proxies.count == 1 && cfg.profiles[0].proxyID == cfg.proxies[0].id, "shared proxy reference")
 let proxy = cfg.proxies[0].settings
 try checked(try SessionCipher.decrypt(proxy.encryptedPassword!,master:master,profile:proxy.credentialProfile) == "fixture-proxy", "proxy password")
 try checked(cfg.sessionLinks.entries.count == 1 && cfg.sessionLinks.entries[0].profileID == cfg.profiles[0].id, "link identity")
 try checked(cfg.quickCommands.first?.text == "echo 互通" && cfg.sessionDefaults.sshPort == 2222, "commands/defaults")
 try checked(cfg.preferences.quickSendScope == .selectedGroups && cfg.preferences.terminalClockOpacity == 0.35, "preferences")
 try checked(cfg.profiles[0].tunnels.count == 1, "tunnels")
 for p in cfg.profiles {
  if p.kind.usesSSH {
   try checked(p.sshAuthenticationMethods == [.gssapi, .keyboardInteractive, .publicKey, .password], "authentication policy round trip")
  }
  try checked(try SessionCipher.decrypt(p.encryptedPassword!,master:master,profile:p) == "fixture-"+p.kind.rawValue, "password round trip")
 }
}
if args[1] == "contract" {
 try SharedConfigurationProtocol.contractData.write(to:URL(fileURLWithPath:args[2]))
} else if args[1] == "archive-roundtrip" {
 do {
  let archive = try SessionArchive.decode(Data(contentsOf:URL(fileURLWithPath:args[2])))
  try archive.encoded().write(to:URL(fileURLWithPath:args[3]))
 } catch {fputs("Session archive rejected input\n",stderr);exit(2)}
} else if args[1] == "canonical-json" || args[1] == "model-json" {
 do {
  let input = try Data(contentsOf:URL(fileURLWithPath:args[2]))
  let normalized = try SharedConfigurationProtocol.canonicalData(input)
  let output: Data
  if args[1] == "model-json" {
   let config = try JSONDecoder().decode(Configuration.self,from:normalized)
   output = try SharedConfigurationProtocol.canonicalData(JSONEncoder().encode(config))
  } else {output=normalized}
  try output.write(to:URL(fileURLWithPath:args[3]))
 } catch {fputs("Shared protocol rejected input\n",stderr);exit(2)}
} else if args[1] == "create" {
 var profiles: [SessionProfile] = []
 for kind in [SessionKind.ssh,.sftp,.ftp] {
  var p = SessionProfile(name: "跨端-"+kind.rawValue,group:"共享/生产",kind:kind,host:"fixture.invalid",port:kind == .ftp ? 21:22,username:"fixture")
  if kind.usesSSH { p.sshAuthenticationMethods = [.gssapi, .keyboardInteractive, .publicKey, .password] }
  p.encryptedPassword = try SessionCipher.encrypt("fixture-"+kind.rawValue,master:master,profile:p,identity:SSHIdentity(host:p.host,user:p.username,port:p.port))
  profiles.append(p)
 }
 var cfg = Configuration(profiles:profiles)
 var proxy = ProxySettings(); proxy.kind = .jump; proxy.host = "jump.invalid"; proxy.port = 22; proxy.username = "jump-user"
 proxy.encryptedPassword = try SessionCipher.encrypt("fixture-proxy",master:master,profile:proxy.credentialProfile,identity:SSHIdentity(host:proxy.host,user:proxy.username,port:proxy.port))
 cfg.proxies = [ProxyProfile(name:"跳板",settings:proxy)]
 cfg.profiles[0].proxyID = proxy.id
 cfg.profiles[0].tunnels = [TunnelRule()]
 cfg.sessionLinks.entries = [SessionLink(profileID:profiles[0].id,name:"快捷引用",folder:"生产")]
 cfg.sessionLinks.folders = ["生产"]
 cfg.quickCommands = [QuickCommand(name:"检查",text:"echo 互通")]
 cfg.sessionDefaults.sshPort = 2222
 cfg.preferences.quickSendScope = .selectedGroups
 cfg.preferences.terminalClockEnabled = true
 cfg.preferences.terminalClockOpacity = 0.35
 cfg.directories = ["共享","共享/生产","空目录"]
 cfg.masterPasswordVerifier = try MasterPasswordProtection.createVerifier(master)
 try SharedVault.encode(cfg,password:master).write(to:URL(fileURLWithPath:args[2]))
} else if args[1] == "directory-edit" || args[1] == "webdav-edit" {
 let transport: SharedSyncTransport
 if args[1] == "directory-edit" { transport = DirectorySyncClient(directory:URL(fileURLWithPath:args[2])) }
 else { transport = try WebDAVClient(WebDAVSettings(address:args[2],username:"test-user",password:"test-password"),allowLoopbackHTTP:true) }
 let old = try transport.fetch()!
 let compatibility = try SharedVault.decode(old.data,password:master); try validate(compatibility)
 var local = compatibility
 local.preferences.fontSize = 21
 local.sessionDefaults.sshPort = 2221
 let applied = SessionSyncConfiguration.applying(compatibility, to: local)
 try checked(applied.preferences.fontSize == 21 && applied.sessionDefaults.sshPort == 2221, "sync changed native local settings")
 var cfg = try SessionSyncConfiguration.project(applied)
 cfg.profiles[0].name = "Swift 修改"
 let wire = try SessionSyncConfiguration.wireConfiguration(cfg, preserving: compatibility)
 let next = try SharedVault.encode(wire,password:master)
 try transport.put(next,matching:old.etag)
 do { try transport.put(old.data,matching:old.etag); throw NSError(domain:"accepted stale write",code:1) }
 catch WebDAVFailure.conflict {}
 print("Swift transport edited shared data and rejected stale write")
} else {
 var cfg = try SharedVault.decode(Data(contentsOf: URL(fileURLWithPath:args[2])),password:master)
 if args[1] != "native-roundtrip" {try validate(cfg)}
 if args[1] == "edit" || args[1] == "roundtrip" || args[1] == "native-roundtrip" {
  if args[1] == "edit" {cfg.profiles[0].name = "Swift 修改"}
  try SharedVault.encode(cfg,password:master).write(to: URL(fileURLWithPath:args[3]))
 }
 print("Swift verified shared configuration and credentials")
}
