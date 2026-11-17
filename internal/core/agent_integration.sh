__oshell_mcp_supported=1;
if [ -n "${BASH_VERSION-}" ]; then
    case "$(declare -p PROMPT_COMMAND 2>/dev/null)" in 'declare -'*r*' PROMPT_COMMAND='*) __oshell_mcp_supported=0;; esac;
    if [ "${BASH_VERSINFO[0]}" -lt 4 ] || { [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -lt 4 ]; }; then
        case "$(trap -p DEBUG)" in ''|*__oshell_mcp_armed*) ;; *) __oshell_mcp_supported=0;; esac;
    fi;
elif [ -z "${ZSH_VERSION-}" ]; then __oshell_mcp_supported=0;
fi;
if [ "$__oshell_mcp_supported" = 1 ]; then
    __oshell_mcp_token=@TOKEN@; __oshell_mcp_initializing=1; __oshell_mcp_armed=0;
    __oshell_mcp_attach() { __oshell_mcp_token=$1; __oshell_mcp_initializing=1; __oshell_mcp_armed=0; };
    __oshell_mcp_version=2;
    __oshell_mcp_start() { builtin printf '\033]779;OSHELL;%s;start\007' "$__oshell_mcp_token"; };
    __oshell_mcp_prompt() {
        local __oshell_mcp_status=$?;
        __oshell_mcp_armed=0;
        if [ "$__oshell_mcp_initializing" = 1 ]; then
            __oshell_mcp_initializing=0;
            builtin printf '\033]779;OSHELL;%s;ready\007' "$__oshell_mcp_token";
        else builtin printf '\033]779;OSHELL;%s;end;%s\007' "$__oshell_mcp_token" "$__oshell_mcp_status";
        fi;
        return "$__oshell_mcp_status";
    };
    __oshell_mcp_arm() { __oshell_mcp_armed=1; };
    if [ -n "${ZSH_VERSION-}" ]; then
        __oshell_mcp_zsh_prompt() { __oshell_mcp_prompt; return 0; };
        preexec_functions=(__oshell_mcp_start "${preexec_functions[@]:#__oshell_mcp_start}");
        precmd_functions=(__oshell_mcp_zsh_prompt "${precmd_functions[@]:#__oshell_mcp_zsh_prompt}");
    else
        if [ "${BASH_VERSINFO[0]}" -gt 4 ] || { [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -ge 4 ]; }; then
            case "${PS0-}" in *'$( __oshell_mcp_start )'*) ;; *) PS0="${PS0-}"'$( __oshell_mcp_start )';; esac;
        else
            trap 'if [ "${__oshell_mcp_armed-0}" = 1 ] && [ "$BASH_COMMAND" != __oshell_mcp_prompt ]; then __oshell_mcp_armed=0; __oshell_mcp_start; fi' DEBUG;
        fi;
        case "${PROMPT_COMMAND[*]-}" in *__oshell_mcp_prompt*) ;;
            *)
                if [ "${BASH_VERSINFO[0]}" -gt 5 ] || { [ "${BASH_VERSINFO[0]}" -eq 5 ] && [ "${BASH_VERSINFO[1]}" -ge 1 ]; }; then
                    PROMPT_COMMAND=(__oshell_mcp_prompt "${PROMPT_COMMAND[@]}" __oshell_mcp_arm);
                else PROMPT_COMMAND="__oshell_mcp_prompt${PROMPT_COMMAND:+$'\n'$PROMPT_COMMAND}"$'\n''__oshell_mcp_arm';
                fi;
                ;;
        esac;
    fi;
else builtin printf '\033]779;OSHELL;%s;unsupported\007' @TOKEN@;
fi
