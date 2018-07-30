local filepath = require("filepath")

function process(event)
  source = event:Get("source")
  if source ~= nil then
    event:Put("file.path", source)
    event:Put("file.name", filepath.base(source))
    event:Put("file.ext", filepath.ext(source))
  end
end
