# Claude Code Transform
# Transforms canonical artifact frontmatter to Claude's expected format

def transform(input, ctx):
    """
    Transform canonical frontmatter for Claude Code harness.

    Args:
        input: Canonical frontmatter dict
        ctx: Harness config from TOML

    Returns:
        {frontmatter, files, warnings}

    Note: henia is available as a pre-loaded global for helpers like henia.dict.get_path()
    """
    out = {"frontmatter": {}, "files": {}, "warnings": []}
    fm = out["frontmatter"]

    # Required: name
    fm["name"] = input.get("name", "")

    # Optional: description
    if "description" in input:
        fm["description"] = input["description"]

    # Model tier -> concrete model via variables
    tier = input.get("model_tier")
    if tier:
        var_key = "model_" + tier
        model = ctx["variables"].get(var_key)
        if model:
            fm["model"] = model
        else:
            out["warnings"].append("Unknown model_tier: " + tier)

    # Tools: map canonical names to Claude names
    tools = input.get("tools", [])
    if tools:
        mapped = []
        for t in tools:
            claude_name = ctx["tools"].get(t, t)  # Default to original if no mapping
            mapped.append(claude_name)
        fm["tools"] = mapped

    # Pass through enabled
    if "enabled" in input:
        fm["enabled"] = input["enabled"]

    # Pass through user_invocable
    if "user_invocable" in input:
        fm["user_invocable"] = input["user_invocable"]

    return out
