# OpenCode Transform
# Transforms canonical artifact frontmatter to OpenCode's expected format
# Also handles permission file generation (opencode.json)

def transform(input, ctx):
    """
    Transform canonical frontmatter for OpenCode harness.

    Args:
        input: Canonical frontmatter dict
        ctx: Harness config from TOML

    Returns:
        {frontmatter, files, warnings}

    Note: henia is available as a pre-loaded global for helpers like henia.dict.get_path()
    """
    out = {"frontmatter": {}, "files": {}, "warnings": []}
    fm = out["frontmatter"]

    # Required: name (OpenCode uses lowercase)
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

    # Build permission dict for opencode.json
    tools = input.get("tools", [])
    policy = input.get("tools_policy", {})  # Optional per-tool policy

    if tools or policy:
        permission = {}

        # From tools list (allow all listed)
        for t in tools:
            opencode_name = ctx["tools"].get(t, t)
            permission[opencode_name] = "allow"

        # From explicit policy
        for t, p in policy.items():
            opencode_name = ctx["tools"].get(t, t)
            permission[opencode_name] = p

        # CRITICAL: OpenCode coupling rule
        # write/patch permissions require edit permission
        if permission.get("write") == "allow" or permission.get("patch") == "allow":
            permission["edit"] = "allow"

        # Emit opencode.json
        out["files"]["opencode.json"] = {"permission": permission}

    return out
