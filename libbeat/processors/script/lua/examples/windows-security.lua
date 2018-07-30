local filepath = require("filepath")

function process(event)
  if event:Get("log_name") ~= "Security" then return end

  processName = event:Get("event_data.ProcessName")
  if processName ~= nil then
    event:Put("process.name", filepath.base(processName))
  end

  eventID = event:Get("event_id")
  if 4756 == eventID then
    event:Rename("event_data.SubjectUserSid",    "user.id")
    event:Rename("event_data.SubjectUserName",   "user.name")
    event:Rename("event_data.SubjectDomainName", "user.domain")
    event:Rename("event_data.SubjectLogonId",    "user.id")
    event:Rename("event_data.SourceIP",          "source.ip")
  elseif 4616 == eventID then
    event:Rename("event_data.SubjectUserSid",    "user.id")
    event:Rename("event_data.ProcessID",         "process.pid")
  end
end
