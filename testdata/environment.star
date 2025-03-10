def main(ctx):
  return [{
    "name": ctx.repo.name,
    "title": ctx.build.title,
    "false": False,
    "true": True,
    "none": None,
  }]
