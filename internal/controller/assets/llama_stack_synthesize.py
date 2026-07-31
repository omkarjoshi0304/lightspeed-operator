#!/usr/bin/env python3

#
# Copyright 2026.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Synthesize OGX (Llama Stack) configuration from lightspeed-stack.yaml.

Runs as an init container using the llama_stack_configuration module
shipped in the lcore image. Reads the operator-generated lightspeed-stack.yaml,
calls synthesize_to_file() which:
  1. Loads the built-in default_run.yaml baseline
  2. Applies enrichments (Solr/OKP, BYOK RAG, Azure Entra ID)
  3. Applies high-level inference providers
  4. Deep-merges native_override (operator-specific: postgres, server, etc.)
  5. Writes the final OGX config to the output path

Usage:
  python3 llama_stack_synthesize.py <lightspeed-stack.yaml> <output-ogx-config.yaml>
"""

import logging
import sys
from pathlib import Path

import yaml
from llama_stack_configuration import synthesize_to_file

logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
logger = logging.getLogger(__name__)


def main():
    if len(sys.argv) != 3:
        print(
            f"Usage: {sys.argv[0]} <lightspeed-stack.yaml> <output-ogx-config.yaml>",
            file=sys.stderr,
        )
        sys.exit(1)

    config_path = sys.argv[1]
    output_path = sys.argv[2]

    logger.info("Reading config from %s", config_path)
    with open(config_path, "r", encoding="utf-8") as f:
        config = yaml.safe_load(f)

    config_dir = str(Path(config_path).parent)

    logger.info("Synthesizing OGX config to %s", output_path)
    synthesize_to_file(config, output_path, config_file_dir=config_dir)

    # Post-process: register the configured LLM model in registered_resources
    # so it's available even when the provider endpoint is unreachable at startup
    # (llama-stack auto-discovery requires a live connection to list models).
    inference = config.get("inference", {})
    default_model = inference.get("default_model")
    default_provider = inference.get("default_provider")
    if default_model and default_provider:
        with open(output_path, "r", encoding="utf-8") as f:
            ogx = yaml.safe_load(f)
        models = ogx.setdefault("registered_resources", {}).setdefault("models", [])
        already = any(m.get("model_id") == default_model for m in models)
        if not already:
            ogx_provider_id = default_provider
            for p in ogx.get("providers", {}).get("inference", []):
                allowed = p.get("config", {}).get("allowed_models", [])
                if default_model in allowed:
                    ogx_provider_id = p.get("provider_id", default_provider)
                    break
            models.append(
                {
                    "model_id": default_model,
                    "model_type": "llm",
                    "provider_id": ogx_provider_id,
                    "provider_model_id": default_model,
                }
            )
            with open(output_path, "w", encoding="utf-8") as f:
                yaml.dump(ogx, f, default_flow_style=False, sort_keys=False)
            logger.info(
                "Registered LLM model %s for provider %s",
                default_model,
                ogx_provider_id,
            )

    logger.info("Done")


if __name__ == "__main__":
    main()
