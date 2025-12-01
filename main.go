package main

import (
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strconv"

    goyaml "gopkg.in/yaml.v3"
)

type ValidationError struct {
    Msg  string
    Line int
}

func main() {
    if len(os.Args) != 2 {
        fmt.Println("usage: yamlvalidator <file>")
        os.Exit(2)
    }
    path := os.Args[1]
    file := filepath.Base(path)

    b, err := os.ReadFile(path)
    if err != nil {
        fmt.Printf("%s: %s\n", file, err)
        os.Exit(2)
    }

    var root goyaml.Node
    if err := goyaml.Unmarshal(b, &root); err != nil {
        fmt.Printf("%s: %s\n", file, err)
        os.Exit(2)
    }

    if len(root.Content) == 0 {
        fmt.Printf("%s: document is empty\n", file)
        os.Exit(1)
    }

    errs := validatePod(root.Content[0])
    if len(errs) > 0 {
        for _, e := range errs {
            if e.Line > 0 {
                fmt.Printf("%s:%d %s\n", file, e.Line, e.Msg)
            } else {
                fmt.Printf("%s: %s\n", file, e.Msg)
            }
        }
        os.Exit(1)
    }

    os.Exit(0)
}

func getMapValue(n *goyaml.Node, key string) *goyaml.Node {
    if n == nil || n.Kind != goyaml.MappingNode {
        return nil
    }
    for i := 0; i < len(n.Content)-1; i += 2 {
        if n.Content[i].Value == key {
            return n.Content[i+1]
        }
    }
    return nil
}

func validatePod(n *goyaml.Node) []ValidationError {
    var r []ValidationError

    nAPI := getMapValue(n, "apiVersion")
    if nAPI == nil {
        r = append(r, ValidationError{"apiVersion is required", 0})
    } else if nAPI.Value != "v1" {
        r = append(r, ValidationError{fmt.Sprintf("apiVersion has unsupported value '%s'", nAPI.Value), nAPI.Line})
    }

    nKind := getMapValue(n, "kind")
    if nKind == nil {
        r = append(r, ValidationError{"kind is required", 0})
    } else if nKind.Value != "Pod" {
        r = append(r, ValidationError{fmt.Sprintf("kind has unsupported value '%s'", nKind.Value), nKind.Line})
    }

    nMeta := getMapValue(n, "metadata")
    if nMeta == nil {
        r = append(r, ValidationError{"metadata is required", 0})
    } else {
        r = append(r, validateMetadata(nMeta)...)
    }

    nSpec := getMapValue(n, "spec")
    if nSpec == nil {
        r = append(r, ValidationError{"spec is required", 0})
    } else {
        r = append(r, validateSpec(nSpec)...)
    }

    return r
}

func validateMetadata(n *goyaml.Node) []ValidationError {
    var r []ValidationError
    nName := getMapValue(n, "name")
    if nName == nil || nName.Value == "" {
        line := 0
        if nName != nil {
            line = nName.Line
        }
        r = append(r, ValidationError{"name is required", line})
    }
    return r
}

func validateSpec(n *goyaml.Node) []ValidationError {
    var r []ValidationError

    nOS := getMapValue(n, "os")
    if nOS != nil && nOS.Value != "linux" && nOS.Value != "windows" {
        r = append(r, ValidationError{fmt.Sprintf("os has unsupported value '%s'", nOS.Value), nOS.Line})
    }

    nCont := getMapValue(n, "containers")
    if nCont == nil || nCont.Kind != goyaml.SequenceNode {
        r = append(r, ValidationError{"spec.containers is required", 0})
        return r
    }

    seen := map[string]bool{}
    for _, c := range nCont.Content {
        r = append(r, validateContainer(c, seen)...)
    }

    return r
}

func validateContainer(n *goyaml.Node, seen map[string]bool) []ValidationError {
    var r []ValidationError

    nName := getMapValue(n, "name")
    if nName == nil || nName.Value == "" {
        line := 0
        if nName != nil {
            line = nName.Line
        }
        r = append(r, ValidationError{"name is required", line})
    } else if seen[nName.Value] {
        r = append(r, ValidationError{fmt.Sprintf("containers.name has invalid format '%s'", nName.Value), nName.Line})
    } else {
        seen[nName.Value] = true
    }

    nImg := getMapValue(n, "image")
    if nImg == nil {
        r = append(r, ValidationError{"image is required", 0})
    } else if !regexp.MustCompile(`^registry\.bigbrother\.io\/[^:]+:[^:]+$`).MatchString(nImg.Value) {
        r = append(r, ValidationError{fmt.Sprintf("image has invalid format '%s'", nImg.Value), nImg.Line})
    }

    nPorts := getMapValue(n, "ports")
    if nPorts != nil && nPorts.Kind == goyaml.SequenceNode {
        for _, p := range nPorts.Content {
            r = append(r, validateContainerPort(p)...)
        }
    }

    for _, pname := range []string{"readinessProbe", "livenessProbe"} {
        pn := getMapValue(n, pname)
        if pn != nil {
            r = append(r, validateProbe(pn, pname)...)
        }
    }

    nRes := getMapValue(n, "resources")
    if nRes == nil {
        r = append(r, ValidationError{"resources is required", 0})
    } else {
        r = append(r, validateResources(nRes)...)
    }

    return r
}

func validateContainerPort(n *goyaml.Node) []ValidationError {
    var r []ValidationError

    cp := getMapValue(n, "containerPort")
    if cp == nil {
        r = append(r, ValidationError{"containerPort is required", 0})
    } else if cp.Tag != "!!int" {
        r = append(r, ValidationError{"containerPort must be int", cp.Line})
    } else {
        v, _ := strconv.Atoi(cp.Value)
        if v <= 0 || v >= 65536 {
            r = append(r, ValidationError{"containerPort value out of range", cp.Line})
        }
    }

    proto := getMapValue(n, "protocol")
    if proto != nil && proto.Value != "TCP" && proto.Value != "UDP" {
        r = append(r, ValidationError{fmt.Sprintf("protocol has unsupported value '%s'", proto.Value), proto.Line})
    }

    return r
}

func validateProbe(n *goyaml.Node, parent string) []ValidationError {
    var r []ValidationError

    hg := getMapValue(n, "httpGet")
    if hg == nil {
        r = append(r, ValidationError{fmt.Sprintf("%s.httpGet is required", parent), 0})
        return r
    }

    p := getMapValue(hg, "path")
    if p == nil || len(p.Value) == 0 || p.Value[0] != '/' {
        line := 0
        if p != nil {
            line = p.Line
        }
        r = append(r, ValidationError{fmt.Sprintf("%s.httpGet.path has invalid format '%s'", parent, p.Value), line})
    }

    port := getMapValue(hg, "port")
    if port == nil || port.Tag != "!!int" {
        line := 0
        if port != nil {
            line = port.Line
        }
        r = append(r, ValidationError{fmt.Sprintf("%s.httpGet.port must be int", parent), line})
    } else {
        v, _ := strconv.Atoi(port.Value)
        if v <= 0 || v >= 65536 {
            r = append(r, ValidationError{fmt.Sprintf("%s.httpGet.port value out of range", parent), port.Line})
        }
    }

    return r
}

func validateResources(n *goyaml.Node) []ValidationError {
    var r []ValidationError

    for _, section := range []string{"requests", "limits"} {
        ns := getMapValue(n, section)
        if ns == nil {
            continue
        }

        cpu := getMapValue(ns, "cpu")
        if cpu != nil && cpu.Tag != "!!int" {
            r = append(r, ValidationError{"cpu must be int", cpu.Line})
        }

        mem := getMapValue(ns, "memory")
        if mem != nil && !regexp.MustCompile(`^\d+(Ki|Mi|Gi)$`).MatchString(mem.Value) {
            r = append(r, ValidationError{fmt.Sprintf("memory has invalid format '%s'", mem.Value), mem.Line})
        }
    }

    return r
}
