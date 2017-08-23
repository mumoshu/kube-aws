#!/usr/bin/env bash

set -ve

echo hello

# Requires the following command beforehand:
#   $ gem install travis
#   $ travis login --auto
#   $ travis encrypt-file ci/kube-aws-bot-git-ssh-key --repo <your github user or organization>/kube-aws

# And then change this line to the one output from the `travis encrypt-file` command above
openssl aes-256-cbc -K $encrypted_514cf8442810_key -iv $encrypted_514cf8442810_iv -in kube-aws-bot-git-ssh-key.enc -out ci/kube-aws-bot-git-ssh-key -d

# Finally,
#   $ git add kube-aws-bot-git-ssh-key.enc
#   $   $ git commit -m '...'

echo -e "Host github.com\n\tStrictHostKeyChecking no\nIdentityFile $(pwd)/ci/kube-aws-bot-git-ssh-key\n" >> ~/.ssh/config

ssh git@github.com

make publish-docs
