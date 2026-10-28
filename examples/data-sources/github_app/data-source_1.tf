data "github_app" "foobar" {
  slug = "foobar"
}

output "app_details" {
  value = {
    client_id   = data.github_app.foobar.client_id
    html_url    = data.github_app.foobar.html_url
    owner       = data.github_app.foobar.owner
    permissions = data.github_app.foobar.permissions
    events      = data.github_app.foobar.events
  }
}
